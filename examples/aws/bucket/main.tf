provider "aws" {
  region = "eu-west-1"
}

resource "aws_s3_bucket" "backup_bucket" {
  bucket = "testbucket"
}

resource "aws_s3_bucket_policy" "backup_bucket_policy" {
  bucket = aws_s3_bucket.backup_bucket.bucket

  policy = <<POLICY
{
  "Version": "2012-10-17",
  "Id": "MYBUCKETPOLICY",
  "Statement": [
  ]
}
POLICY
}

resource "aws_iam_user" "backup_user" {
  name  = "backup-user"
  tags = {
    Name = "Backup"
  }
}

resource "aws_iam_user_policy" "backup_policy" {
  name  = "backup-policy"
  user  = aws_iam_user.backup_user.name

  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Action": [
        "s3:*"
      ],
      "Effect": "Allow",
      "Resource":[
        "${aws_s3_bucket.backup_bucket.arn}/*"
      ]
    }
  ]
}
EOF
}

output "bucket_id" {
  value = aws_s3_bucket.backup_bucket.id
}
