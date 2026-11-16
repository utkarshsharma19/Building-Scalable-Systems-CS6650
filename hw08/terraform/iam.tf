data "aws_iam_policy_document" "ecs_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

# Execution role: lets ECS pull the image and ship logs.
resource "aws_iam_role" "execution" {
  name               = "${var.name}-task-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
}

resource "aws_iam_role_policy_attachment" "execution" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# Task role: what the Go service itself may do - only these calls, only on this table.
resource "aws_iam_role" "task" {
  name               = "${var.name}-cart-task"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
}

data "aws_iam_policy_document" "task" {
  statement {
    actions = ["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:UpdateItem", "dynamodb:Query", "dynamodb:DescribeTable"]
    resources = [
      aws_dynamodb_table.carts.arn,
      "${aws_dynamodb_table.carts.arn}/index/*",
    ]
  }
}

resource "aws_iam_role_policy" "task" {
  name   = "dynamodb-carts"
  role   = aws_iam_role.task.id
  policy = data.aws_iam_policy_document.task.json
}
