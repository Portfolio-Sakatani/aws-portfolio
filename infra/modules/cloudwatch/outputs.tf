output "log_group_name" {
  value       = aws_cloudwatch_log_group.this.name
  description = "作成されたロググループ名"
}