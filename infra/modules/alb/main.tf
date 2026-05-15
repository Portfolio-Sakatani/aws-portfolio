# ALB定義
resource "aws_lb" "this" {
  name               = "${var.project}-alb"
  load_balancer_type = "application"
  subnets            = var.public_subnets
  security_groups    = [var.alb_sg_id]

  # クロスゾーン負荷分散を明示的に有効（全ターゲットへ均等に振るため）
  enable_cross_zone_load_balancing = true
}

# ターゲットグループ定義
resource "aws_lb_target_group" "this" {
  name        = "${var.project}-tg"
  port        = 8080
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip"

  # 振り分けアルゴリズムを明示的に指定
  load_balancing_algorithm_type = "round_robin"

  # 2. スティッキーセッションを無効化（交代を妨げない）
  stickiness {
    enabled = false
    type    = "lb_cookie"
  }

# ヘルスチェック設定
  health_check {
    path                = "/"
    matcher             = "200"
    interval            = 15 
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 2
  }

# 新しいリソースを作成してから旧リソースを削除する
  lifecycle {
    create_before_destroy = true
  }
}

# リスナー設定
resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.this.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.this.arn
  }
}
