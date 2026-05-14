variable "project"            { type = string }
variable "private_subnets"    { type = list(string) }
variable "ecs_sg_id"          { type = string }
variable "execution_role_arn" { type = string }
variable "target_group_arn"   { type = string }
variable "container_image"    { type = string }
variable "log_group_name"     { type = string }
variable "region"             { 
  type    = string
  default = "ap-northeast-1"
}