# ECSタスク実行ロール定義
resource "aws_iam_role" "ecs_task_execution" {
  name = "${var.project}-ecs-exec-role"

# ECSタスクへの使用許可
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Service = "ecs-tasks.amazonaws.com"
      }
      Action = "sts:AssumeRole"
    }]
  })
}

# 作成したIAMロールに、AWS管理の「AmazonECSTaskExecutionRolePolicy」をアタッチ(ECRからのイメージ取得やCloudWatchへのログ出力が可能に)
resource "aws_iam_role_policy_attachment" "ecs_exec_attach" {
  role       = aws_iam_role.ecs_task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}
