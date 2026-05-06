package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
)

// -------------------------------------------------------
// 共通型定義
// -------------------------------------------------------

// PolicyDocument は SCP ポリシーの JSON 構造体です
type PolicyDocument struct {
	Version   string      `json:"Version"`
	Statement []Statement `json:"Statement"`
}

// Statement は SCP ポリシーの Statement 要素です
// Action / Resource は string または []string を許容するため any を使います
type Statement struct {
	Sid      string `json:"Sid,omitempty"`
	Effect   string `json:"Effect"`
	Action   any    `json:"Action"`
	Resource any    `json:"Resource"`
}

// -------------------------------------------------------
// main
// -------------------------------------------------------

func main() {
	ctx := context.Background()

	// AWS 設定の読み込み（~/.aws/credentials または環境変数を使用）
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("AWS 設定の読み込みに失敗しました: %v", err)
	}

	client := organizations.NewFromConfig(cfg)

	fmt.Println("========================================================")
	fmt.Println(" AWS Organizations OU 構成セットアップ")
	fmt.Println("  Root")
	fmt.Println("  ├── OU: Dev        （削除制限なし）")
	fmt.Println("  └── OU: Production （s3:DeleteBucket 禁止 SCP 適用）")
	fmt.Println("========================================================")

	// ----------------------------------------------------------
	// Step 1: 組織の確認 / 有効化
	// ----------------------------------------------------------
	fmt.Println("\n[Step 1] 組織の確認 / 有効化")
	if err := ensureOrganization(ctx, client); err != nil {
		log.Fatalf("組織の確認/有効化に失敗しました: %v", err)
	}

	// ----------------------------------------------------------
	// Step 2: Root ID の取得
	// ----------------------------------------------------------
	fmt.Println("\n[Step 2] Root ID の取得")
	rootID, err := getRootID(ctx, client)
	if err != nil {
		log.Fatalf("Root ID の取得に失敗しました: %v", err)
	}
	fmt.Printf("  ✅ Root ID: %s\n", rootID)

	// ----------------------------------------------------------
	// Step 3: Dev OU の作成
	//   UseCase B: 開発環境の柔軟さ
	//   Dev OU には Deny SCP を適用しない。
	//   開発者は S3 バケットを自由に作成・変更・削除できる。
	// ----------------------------------------------------------
	fmt.Println("\n[Step 3] Dev OU の作成（UseCase B: 削除制限なし）")
	devOUID, err := ensureOU(ctx, client, rootID, "Dev")
	if err != nil {
		log.Fatalf("Dev OU の作成に失敗しました: %v", err)
	}
	fmt.Printf("  ✅ Dev OU ID: %s\n", devOUID)
	fmt.Println("  ℹ️  Dev OU には Deny SCP を適用しません。")
	fmt.Println("     開発者はバケットを自由に作成・削除できます。")

	// ----------------------------------------------------------
	// Step 4: Production OU の作成
	// ----------------------------------------------------------
	fmt.Println("\n[Step 4] Production OU の作成")
	prodOUID, err := ensureOU(ctx, client, rootID, "Production")
	if err != nil {
		log.Fatalf("Production OU の作成に失敗しました: %v", err)
	}
	fmt.Printf("  ✅ Production OU ID: %s\n", prodOUID)

	// ----------------------------------------------------------
	// Step 5: Production OU へ SCP を適用
	//   UseCase A: 本番環境の保護（S3 バケット削除防止）
	//   SCP は「最大許可範囲」を制限するため、
	//   IAM で AdministratorAccess が付いていても s3:DeleteBucket が禁止されます。
	// ----------------------------------------------------------
	fmt.Println("\n[Step 5] Production OU への SCP 適用（UseCase A: s3:DeleteBucket 禁止）")
	if err := applyDenyDeleteBucketSCP(ctx, client, prodOUID); err != nil {
		log.Fatalf("Production SCP の適用に失敗しました: %v", err)
	}

	// ----------------------------------------------------------
	// 完了サマリー
	// ----------------------------------------------------------
	fmt.Println("\n========================================================")
	fmt.Println(" 🎉 セットアップ完了")
	fmt.Println("========================================================")
	fmt.Println("  組織構成:")
	fmt.Println("  Root")
	fmt.Printf("  ├── OU: Dev    (ID: %s)\n", devOUID)
	fmt.Println("  │     SCP: なし（バケット作成・削除・変更すべて許可）")
	fmt.Printf("  └── OU: Production (ID: %s)\n", prodOUID)
	fmt.Println("        SCP: DenyS3DeleteBucket（s3:DeleteBucket を全員に禁止）")
	fmt.Println()
	fmt.Println("  継承ルール:")
	fmt.Println("  SCP は Production OU に属するすべてのアカウントに継承されます。")
	fmt.Println("  IAM で AdministratorAccess が付いていても削除は拒否されます。")
}

// -------------------------------------------------------
// 組織管理
// -------------------------------------------------------

// ensureOrganization は組織が存在しない場合に新規作成します。
// すでに存在する場合はスキップします。
func ensureOrganization(ctx context.Context, client *organizations.Client) error {
	_, err := client.DescribeOrganization(ctx, &organizations.DescribeOrganizationInput{})
	if err == nil {
		fmt.Println("  ℹ️  組織は既に存在します。")
		return nil
	}

	_, err = client.CreateOrganization(ctx, &organizations.CreateOrganizationInput{
		// ALL: SCP / Tag Policy / AI オプトアウトポリシーなどすべての機能が有効
		FeatureSet: orgtypes.OrganizationFeatureSetAll,
	})
	if err != nil {
		return fmt.Errorf("組織の作成に失敗: %w", err)
	}
	fmt.Println("  ✅ 組織を作成しました（FeatureSet: ALL）")
	return nil
}

// getRootID は組織の Root ID を取得します。
func getRootID(ctx context.Context, client *organizations.Client) (string, error) {
	output, err := client.ListRoots(ctx, &organizations.ListRootsInput{})
	if err != nil {
		return "", fmt.Errorf("Root 一覧の取得に失敗: %w", err)
	}
	if len(output.Roots) == 0 {
		return "", fmt.Errorf("Root が見つかりません")
	}
	return *output.Roots[0].Id, nil
}

// ensureOU は指定した名前の OU が存在しない場合に作成し、OU ID を返します。
// すでに同名の OU が存在する場合はその ID を返してスキップします。
func ensureOU(ctx context.Context, client *organizations.Client, parentID, ouName string) (string, error) {
	// 既存の OU を検索
	paginator := organizations.NewListOrganizationalUnitsForParentPaginator(client,
		&organizations.ListOrganizationalUnitsForParentInput{
			ParentId: aws.String(parentID),
		},
	)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return "", fmt.Errorf("OU 一覧の取得に失敗: %w", err)
		}
		for _, ou := range page.OrganizationalUnits {
			if *ou.Name == ouName {
				fmt.Printf("  ℹ️  OU '%s' は既に存在します（ID: %s）\n", ouName, *ou.Id)
				return *ou.Id, nil
			}
		}
	}

	// 新規作成
	output, err := client.CreateOrganizationalUnit(ctx, &organizations.CreateOrganizationalUnitInput{
		ParentId: aws.String(parentID),
		Name:     aws.String(ouName),
	})
	if err != nil {
		return "", fmt.Errorf("OU '%s' の作成に失敗: %w", ouName, err)
	}
	fmt.Printf("  ✅ OU を作成しました: %s\n", ouName)
	return *output.OrganizationalUnit.Id, nil
}

// -------------------------------------------------------
// UseCase A: SCP による本番環境保護
// -------------------------------------------------------

// applyDenyDeleteBucketSCP は Production OU に s3:DeleteBucket を禁止する SCP を
// 作成してアタッチします。
//
// SCP の継承ルール:
//   - Production OU に属するすべてのアカウントに適用されます。
//   - IAM で AdministratorAccess が付与されていても本 SCP による Deny が優先されます。
//   - バケットの作成やオブジェクトの読み書きは IAM ポリシーで別途許可できます。
func applyDenyDeleteBucketSCP(ctx context.Context, client *organizations.Client, prodOUID string) error {
	const scpName = "DenyS3DeleteBucket"

	// SCP ポリシードキュメント
	scpDoc := PolicyDocument{
		Version: "2012-10-17",
		Statement: []Statement{
			{
				Sid:      "DenyDeleteBucket",
				Effect:   "Deny",
				Action:   "s3:DeleteBucket",
				Resource: "*",
			},
		},
	}
	docJSON, err := json.Marshal(scpDoc)
	if err != nil {
		return fmt.Errorf("SCP ドキュメントの JSON 変換に失敗: %w", err)
	}

	// 既存の SCP を検索
	scpArn, err := findSCP(ctx, client, scpName)
	if err != nil {
		return err
	}

	if scpArn == "" {
		// SCP の新規作成
		output, err := client.CreatePolicy(ctx, &organizations.CreatePolicyInput{
			Name:        aws.String(scpName),
			Description: aws.String("本番環境での S3 バケット削除を禁止する SCP"),
			Type:        orgtypes.PolicyTypeServiceControlPolicy,
			Content:     aws.String(string(docJSON)),
		})
		if err != nil {
			return fmt.Errorf("SCP '%s' の作成に失敗: %w", scpName, err)
		}
		scpArn = *output.Policy.PolicySummary.Arn
		fmt.Printf("  ✅ SCP を作成しました: %s\n", scpName)
		fmt.Printf("     ARN: %s\n", scpArn)
		fmt.Printf("     内容: %s\n", string(docJSON))
	} else {
		fmt.Printf("  ℹ️  SCP '%s' は既に存在します（ARN: %s）\n", scpName, scpArn)
	}

	// Production OU へのアタッチ
	if err := attachSCP(ctx, client, scpArn, prodOUID); err != nil {
		return err
	}
	fmt.Printf("  ✅ SCP '%s' を Production OU (%s) にアタッチしました\n", scpName, prodOUID)
	return nil
}

// findSCP は指定した名前の SCP を検索し、ARN を返します。
// 見つからない場合は空文字を返します。
func findSCP(ctx context.Context, client *organizations.Client, name string) (string, error) {
	paginator := organizations.NewListPoliciesPaginator(client, &organizations.ListPoliciesInput{
		Filter: orgtypes.PolicyTypeServiceControlPolicy,
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return "", fmt.Errorf("SCP 一覧の取得に失敗: %w", err)
		}
		for _, p := range page.Policies {
			if *p.Name == name {
				return *p.Arn, nil
			}
		}
	}
	return "", nil
}

// attachSCP は SCP を指定ターゲット（OU / アカウント）にアタッチします。
// すでにアタッチ済みの場合はスキップします。
func attachSCP(ctx context.Context, client *organizations.Client, policyArn, targetID string) error {
	_, err := client.AttachPolicy(ctx, &organizations.AttachPolicyInput{
		PolicyId: aws.String(policyArn),
		TargetId: aws.String(targetID),
	})
	if err != nil {
		// DuplicatePolicyAttachmentException はすでにアタッチ済みなのでスキップ
		if containsStr(err.Error(), "DuplicatePolicyAttachmentException") ||
			containsStr(err.Error(), "already attached") {
			fmt.Println("  ℹ️  SCP はすでにアタッチ済みです。")
			return nil
		}
		return fmt.Errorf("SCP のアタッチに失敗: %w", err)
	}
	return nil
}

// -------------------------------------------------------
// ユーティリティ
// -------------------------------------------------------

// containsStr は文字列 s に substr が含まれるかを返します。
func containsStr(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
