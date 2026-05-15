# ECRリポジトリの定義
resource "aws_ecr_repository" "this" {
  name = "${var.project}-app"

  image_scanning_configuration {
    scan_on_push = true
  }

  # リソース削除時の動作設定(強制削除)
  force_delete = true
}
