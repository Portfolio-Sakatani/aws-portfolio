terraform {
  backend "local" {}
}

provider "aws" {
  region = var.region
}

# 1. ランダムな文字列の生成
resource "random_string" "suffix" {
  length  = 6
  special = false # 特殊文字禁止
  upper   = false # 大文字を禁止してS3命名エラーを回避
}

# 2. S3バケット本体
resource "aws_s3_bucket" "tfstate" {
  bucket = "${var.project}-tfstate-${random_string.suffix.result}"

  lifecycle {
    prevent_destroy = true # 誤削除防止
  }

  tags = {
    Name = "${var.project}-tfstate"
  }
}

# 3. バージョニング設定（履歴管理）
resource "aws_s3_bucket_versioning" "tfstate" {
  bucket = aws_s3_bucket.tfstate.id
  versioning_configuration {
    status = "Enabled"
  }
}

# 4. 暗号化設定（セキュリティ）
resource "aws_s3_bucket_server_side_encryption_configuration" "tfstate" {
  bucket = aws_s3_bucket.tfstate.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# 5. パブリックアクセスブロック
resource "aws_s3_bucket_public_access_block" "tfstate" {
  bucket                  = aws_s3_bucket.tfstate.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# 6. 出力（backend.tfに書き込むための値）
output "bucket_name" {
  value = aws_s3_bucket.tfstate.id
}