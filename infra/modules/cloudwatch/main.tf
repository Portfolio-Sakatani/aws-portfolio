resource "aws_cloudwatch_log_group" "this" {
  name              = "/ecs/${var.project}-app"
  retention_in_days = var.retention_days

  tags = {
    Name = "${var.project}-log-group"
  }
}