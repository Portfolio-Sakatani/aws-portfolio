# # ECSクラスター定義
resource "aws_ecs_cluster" "this" {
  name = "${var.project}-cluster"
}

# タスク定義（コンテナの実行スペックや利用イメージの定義）
resource "aws_ecs_task_definition" "this" {
  family                   = "${var.project}-task"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = var.execution_role_arn

# コンテナの詳細設定（JSON形式
  container_definitions = jsonencode([
    {
      name  = "${var.project}-app"
      image = var.container_image

      portMappings = [
        {
          containerPort = 8080
          hostPort      = 8080
        }
      ]

      # アプリケーションに渡す環境変数の設定
      environment = [
        {
          name  = "AWS_REGION"
          value = var.region
        }
      ]

      # CloudWatch Logsへのログ出力設定
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = var.log_group_name
          "awslogs-region"        = var.region
          "awslogs-stream-prefix" = "ecs"
        }
      }
    }
  ])
}

# ECS定義
resource "aws_ecs_service" "this" {
  name            = "${var.project}-service"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.this.arn
  desired_count   = 2
  launch_type     = "FARGATE"

  # ネットワーク設定
  network_configuration {
    subnets          = var.private_subnets
    security_groups  = [var.ecs_sg_id]
    assign_public_ip = false
  }

  # ロードバランサーとの連携設定
  load_balancer {
    target_group_arn = var.target_group_arn
    container_name   = "${var.project}-app"
    container_port   = 8080
  }
}
