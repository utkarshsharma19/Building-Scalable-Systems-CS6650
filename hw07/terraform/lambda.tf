# Part III: SNS -> Lambda, no SQS. Build the zip first: ../scripts/build-lambda.sh
#
# WARNING: never run Locust while lambda_subscribed = true. Every published
# order would start a 3 s Lambda, and load testing Lambda can get the lab
# account deactivated. Use 5-10 curl requests only.
locals {
  lambda_zip = "${path.module}/../build/lambda.zip"
}

resource "aws_cloudwatch_log_group" "lambda" {
  count             = var.deploy_lambda ? 1 : 0
  name              = "/aws/lambda/${var.name}-order-processor"
  retention_in_days = var.log_retention_days
}

resource "aws_lambda_function" "orders" {
  count            = var.deploy_lambda ? 1 : 0
  function_name    = "${var.name}-order-processor"
  role             = data.aws_iam_role.lab_role.arn
  runtime          = "provided.al2"
  handler          = "bootstrap"
  architectures    = ["x86_64"]
  memory_size      = 512
  timeout          = 10
  filename         = local.lambda_zip
  source_code_hash = fileexists(local.lambda_zip) ? filebase64sha256(local.lambda_zip) : null

  environment {
    variables = { PAYMENT_DELAY = var.payment_delay }
  }
  depends_on = [aws_cloudwatch_log_group.lambda]
}

resource "aws_lambda_permission" "sns" {
  count         = var.deploy_lambda ? 1 : 0
  statement_id  = "AllowSNSInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.orders[0].function_name
  principal     = "sns.amazonaws.com"
  source_arn    = aws_sns_topic.orders.arn
}

# Turn this off (lambda_subscribed = false) before any Part II rerun.
resource "aws_sns_topic_subscription" "lambda" {
  count      = var.deploy_lambda && var.lambda_subscribed ? 1 : 0
  topic_arn  = aws_sns_topic.orders.arn
  protocol   = "lambda"
  endpoint   = aws_lambda_function.orders[0].arn
  depends_on = [aws_lambda_permission.sns]
}
