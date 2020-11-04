#!/bin/bash

set -euxo pipefail
source .env
GOOS=linux
BUILD_NAME=${NAME:-"simple-subscribe"}

go build && zip "$BUILD_NAME".zip "$BUILD_NAME"

aws lambda update-function-code \
    --function-name "$BUILD_NAME" \
    --zip-file fileb://"$BUILD_NAME".zip

rm "$BUILD_NAME" "$BUILD_NAME".zip

aws lambda update-function-configuration \
    --function-name "$BUILD_NAME" \
    --environment "$LAMBDA_ENV"
