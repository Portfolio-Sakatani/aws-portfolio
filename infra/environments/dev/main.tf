# --- Terraform Configuration ---
terraform {
  required_version = "~> 1.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }

  # S3バックエンドの設定
  backend "s3" {
    bucket       = "portfolio-tfstate-2p7kv9" 
    key          = "dev/terraform.tfstate" 
    region       = "ap-northeast-1" 
    use_lockfile = true 
    encrypt      = true 
  }
}

# --- Data Sources ---
# 取得済みのホストゾーン情報を参照
data "aws_route53_zone" "main" {
  name         = "aws-infra-portfolio.com."
  private_zone = false
}

# --- Modules ---

# 1. VPCモジュールの呼び出し
module "vpc" {
  source  = "../../modules/vpc"
  project = "portfolio"
}

# 2. SGモジュールの呼び出し（VPC IDが必要）
module "sg" {
  source  = "../../modules/sg"
  project = "portfolio"
  vpc_id  = module.vpc.vpc_id
}

# 3. IAMモジュールの呼び出し
module "iam" {
  source  = "../../modules/iam"
  project = "portfolio"
}

# 4. CloudWatchモジュールの呼び出し
module "cloudwatch" {
  source         = "../../modules/cloudwatch"
  project        = "portfolio"
  retention_days = 7
}

# 5. ECRモジュールの呼び出し
module "ecr" {
  source  = "../../modules/ecr"
  project = "portfolio"
}

# 6. ALBモジュールの呼び出し（SG IDが必要）
module "alb" {
  source          = "../../modules/alb"
  project         = "portfolio"
  vpc_id          = module.vpc.vpc_id
  public_subnets  = module.vpc.public_subnets
  alb_sg_id       = module.sg.alb_sg_id
}

# 7. ECSモジュールの呼び出し
module "ecs" {
  source             = "../../modules/ecs" 
  project            = "portfolio" 
  private_subnets    = module.vpc.private_subnets 
  ecs_sg_id          = module.sg.ecs_sg_id 
  execution_role_arn = module.iam.execution_role_arn 
  target_group_arn   = module.alb.target_group_arn 
  container_image    = "${module.ecr.repository_url}:latest" 
  log_group_name     = module.cloudwatch.log_group_name 
}

# 8. Route 53モジュールの呼び出し
# ALBのDNS名とZone IDを紐付け、ドメインでアクセス可能にする
module "route53" {
  source       = "../../modules/route53" 
  zone_id      = data.aws_route53_zone.main.zone_id 
  domain_name  = "www.${data.aws_route53_zone.main.name}" 
  alb_dns_name = module.alb.alb_dns_name 
  alb_zone_id  = module.alb.alb_zone_id 
}