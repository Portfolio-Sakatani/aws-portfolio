variable "project" {
  type        = string
  description = "プロジェクト名"
}

variable "retention_days" {
  type        = number
  default     = 7
  description = "ログの保持期間（日）"
}