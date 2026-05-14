# VPC関連
output "vpc_id" {
  value = module.vpc.vpc_id
}

# ネットワーク関連
output "public_subnets" {
  value = module.vpc.public_subnets
}

output "private_subnets" {
  value = module.vpc.private_subnets
}

# ALBのエンドポイント（動作確認用）
output "alb_dns_name" {
  description = "The DNS name of the load balancer"
  value       = module.alb.alb_dns_name
}

# ECRのプッシュ先
output "ecr_repository_url" {
  description = "The URL of the ECR repository"
  value       = module.ecr.repository_url
}