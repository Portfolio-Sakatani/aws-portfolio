resource "aws_ecr_repository" "this" {
  name = "${var.project}-app"

  image_scanning_configuration {
    scan_on_push = true
  }

  # 学習用途のため、リポジトリ削除時にイメージが残っていても削除を許可する設定
  force_delete = true
}