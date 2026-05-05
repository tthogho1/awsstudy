# IAM User & Policy Demo (Go)

Overview
- This small Go program demonstrates creating IAM users, attaching managed and custom policies, and creating console login profiles using the AWS SDK for Go v2.
- It programmatically creates two users (`userA`, `userB`), creates two policies (`PolicyRW`, `PolicyR`), and attaches them. It also creates login profiles (passwords) for the users.

Files
- `main.go`: Example program that performs the IAM operations.

Prerequisites
- Go 1.18+ installed.
- AWS SDK for Go v2 (module dependencies are in `go.mod`).
- AWS credentials configured (`~/.aws/credentials` or environment variables).
- The AWS identity used to run the program must have IAM permissions to create users, policies, attach policies, and create login profiles. Example permissions required:
  - `iam:CreateUser`, `iam:CreateLoginProfile`, `iam:CreatePolicy`, `iam:AttachUserPolicy`, `iam:DeleteUser`, `iam:DeletePolicy`, etc.

Configuration notes
- The bucket name used in the example policies is hard-coded as `bucket-for-policytest` in `main.go`. Update the ARN(s) in the source if you want to target a different S3 bucket.

Usage
1. Change into the `iam` folder:

```sh
cd iam
```

2. Run the program (it will create IAM resources in your AWS account):

```sh
go run main.go
```

or build and run:

```sh
go build -o iam && ./iam
```

Safety & cleanup
- This program will create real IAM users, policies, and login profiles in your account. Run it only in a test account or environment.
- To remove created resources, you can delete them via the AWS Console or AWS CLI. Example AWS CLI commands (replace names/ARNs as needed):

```sh
aws iam detach-user-policy --user-name userA --policy-arn arn:aws:iam::123456789012:policy/PolicyRW
aws iam delete-policy --policy-arn arn:aws:iam::123456789012:policy/PolicyRW
aws iam delete-login-profile --user-name userA
aws iam delete-user --user-name userA
```

Notes
- Passwords in the example are set directly in code for demonstration only. Do NOT hard-code secrets in production.
- Review and adapt policy documents in `main.go` to follow least-privilege principles before running in any environment.

License & attribution
- Example code only; adapt and reuse as needed.
