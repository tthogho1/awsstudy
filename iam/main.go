package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
)

// PolicyDocument はIAMポリシーのJSON構造体です
type PolicyDocument struct {
	Version   string      `json:"Version"`
	Statement []Statement `json:"Statement"`
}

type Statement struct {
	Effect   string   `json:"Effect"`
	Action   []string `json:"Action"`
	Resource []string `json:"Resource"`
}

func main() {
	ctx := context.Background()

	// AWS設定の読み込み（~/.aws/credentials または環境変数を使用）
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("AWS設定の読み込みに失敗しました: %v", err)
	}

	client := iam.NewFromConfig(cfg)

	// -------------------------------------------------------
	// 1. ユーザーの作成
	// -------------------------------------------------------
	userA, err := createUser(ctx, client, "userA")
	if err != nil {
		log.Fatalf("userA の作成に失敗しました: %v", err)
	}
	fmt.Printf("✅ ユーザー作成: %s (ARN: %s)\n", *userA.User.UserName, *userA.User.Arn)

	userB, err := createUser(ctx, client, "userB")
	if err != nil {
		log.Fatalf("userB の作成に失敗しました: %v", err)
	}
	fmt.Printf("✅ ユーザー作成: %s (ARN: %s)\n", *userB.User.UserName, *userB.User.Arn)

	// ログインプロファイル作成（マネジメントコンソールログイン用パスワード設定）
	const password = "iamtestpassword123!" // 注意: 実際の運用では安全なパスワード管理を行ってください
	if err = createLoginProfile(ctx, client, *userA.User.UserName, password); err != nil {
		log.Fatalf("userA のログインプロファイル作成に失敗しました: %v", err)
	}
	fmt.Printf("✅ ログインプロファイル作成: %s\n", *userA.User.UserName)

	if err = createLoginProfile(ctx, client, *userB.User.UserName, password); err != nil {
		log.Fatalf("userB のログインプロファイル作成に失敗しました: %v", err)
	}
	fmt.Printf("✅ ログインプロファイル作成: %s\n", *userB.User.UserName)

	// マネジメントコンソールログイン用ポリシーアタッチ（パスワード変更権限）
	const iamUserChangePasswordPolicy = "arn:aws:iam::aws:policy/IAMUserChangePassword"
	for _, userName := range []string{*userA.User.UserName, *userB.User.UserName} {
		if err = attachPolicyToUser(ctx, client, userName, iamUserChangePasswordPolicy); err != nil {
			log.Fatalf("%s への IAMUserChangePassword アタッチに失敗しました: %v", userName, err)
		}
		fmt.Printf("✅ ポリシー割り当て: IAMUserChangePassword → %s\n", userName)
	}

	// -------------------------------------------------------
	// 2. ポリシーの作成
	// -------------------------------------------------------
	policyRWArn, policyRArn, err := setupPolicies(ctx, client)
	if err != nil {
		log.Fatalf("ポリシーの作成に失敗しました: %v", err)
	}

	// -------------------------------------------------------
	// 3. ポリシーをユーザーへ割り当て
	// -------------------------------------------------------
	err = attachPolicyToUser(ctx, client, *userA.User.UserName, policyRWArn)
	if err != nil {
		log.Fatalf("PolicyRW を userA へ割り当てに失敗しました: %v", err)
	}
	fmt.Printf("✅ ポリシー割り当て: PolicyRW → %s\n", *userA.User.UserName)

	err = attachPolicyToUser(ctx, client, *userB.User.UserName, policyRArn)
	if err != nil {
		log.Fatalf("PolicyR を userB へ割り当てに失敗しました: %v", err)
	}
	fmt.Printf("✅ ポリシー割り当て: PolicyR → %s\n", *userB.User.UserName)

	fmt.Println("\n🎉 すべての操作が完了しました。")
}

// createUser は IAM ユーザーを作成して返します
func createUser(ctx context.Context, client *iam.Client, userName string) (*iam.CreateUserOutput, error) {
	output, err := client.CreateUser(ctx, &iam.CreateUserInput{
		UserName: aws.String(userName),
	})
	if err != nil {
		return nil, err
	}
	return output, nil
}

// createLoginProfile はマネジメントコンソールログイン用のパスワードを設定します
func createLoginProfile(ctx context.Context, client *iam.Client, userName, password string) error {
	_, err := client.CreateLoginProfile(ctx, &iam.CreateLoginProfileInput{
		UserName:              aws.String(userName),
		Password:              aws.String(password),
		PasswordResetRequired: true, // 初回ログイン時にパスワード変更を強制
	})
	return err
}

// createPolicy は IAM ポリシーを作成してその ARN を返します
func createPolicy(ctx context.Context, client *iam.Client, name, description string, document PolicyDocument) (string, error) {
	docJSON, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("ポリシードキュメントのJSON変換に失敗: %w", err)
	}

	output, err := client.CreatePolicy(ctx, &iam.CreatePolicyInput{
		PolicyName:     aws.String(name),
		Description:    aws.String(description),
		PolicyDocument: aws.String(string(docJSON)),
	})
	if err != nil {
		return "", err
	}
	return *output.Policy.Arn, nil
}

// attachPolicyToUser は指定ユーザーにポリシーを割り当てます
func attachPolicyToUser(ctx context.Context, client *iam.Client, userName, policyArn string) error {
	_, err := client.AttachUserPolicy(ctx, &iam.AttachUserPolicyInput{
		UserName:  aws.String(userName),
		PolicyArn: aws.String(policyArn),
	})
	return err
}

// setupPolicies は PolicyRW（読み書き）と PolicyR（読み込みのみ）を作成し、それぞれの ARN を返します
func setupPolicies(ctx context.Context, client *iam.Client) (policyRWArn, policyRArn string, err error) {
	// PolicyRW: BucketA への読み書きアクセス
	policyRWDocument := PolicyDocument{
		Version: "2012-10-17",
		Statement: []Statement{
			{
				Effect: "Allow",
				Action: []string{
					"s3:GetObject",
					"s3:PutObject",
					"s3:DeleteObject",
					"s3:ListBucket",
				},
				Resource: []string{
					"arn:aws:s3:::bucket-for-policytest",
					"arn:aws:s3:::bucket-for-policytest/*",
				},
			},
			{
				Effect:   "Allow",
				Action:   []string{"s3:ListAllMyBuckets"},
				Resource: []string{"*"},
			},
		},
	}
	policyRWArn, err = createPolicy(ctx, client, "PolicyRW", "BucketAへの読み書きポリシー", policyRWDocument)
	if err != nil {
		return "", "", fmt.Errorf("PolicyRW の作成に失敗しました: %w", err)
	}
	fmt.Printf("✅ ポリシー作成: PolicyRW (ARN: %s)\n", policyRWArn)

	// PolicyR: BucketA への読み込みのみアクセス
	policyRDocument := PolicyDocument{
		Version: "2012-10-17",
		Statement: []Statement{
			{
				Effect: "Allow",
				Action: []string{
					"s3:GetObject",
					"s3:ListBucket",
				},
				Resource: []string{
					"arn:aws:s3:::bucket-for-policytest",
					"arn:aws:s3:::bucket-for-policytest/*",
				},
			},
			{
				Effect:   "Allow",
				Action:   []string{"s3:ListAllMyBuckets"},
				Resource: []string{"*"},
			},
		},
	}
	policyRArn, err = createPolicy(ctx, client, "PolicyR", "BucketAへの読み込みポリシー", policyRDocument)
	if err != nil {
		return "", "", fmt.Errorf("PolicyR の作成に失敗しました: %w", err)
	}
	fmt.Printf("✅ ポリシー作成: PolicyR (ARN: %s)\n", policyRArn)

	return policyRWArn, policyRArn, nil
}
