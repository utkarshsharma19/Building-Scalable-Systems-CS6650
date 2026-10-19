resource "aws_ecr_repository" "this" {
  name = var.repository_name
  # Allow `terraform destroy` to delete the repo even when it still holds images
  force_delete = true
}
