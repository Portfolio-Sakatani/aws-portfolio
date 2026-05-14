variable "zone_id" {
  type        = string
  description = "Route 53のホストゾーンID"
}

variable "domain_name" {
  type        = string
  description = "ドメイン名（aws-infra-portfolio.com）"
}

variable "alb_dns_name" {
  type        = string
  description = "ALBのDNS名"
}

variable "alb_zone_id" {
  type        = string
  description = "ALBのゾーンID"
}