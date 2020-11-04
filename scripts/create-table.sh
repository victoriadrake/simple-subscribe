#!/bin/bash

set -euxo pipefail
source .env
BUILD_TABLE_NAME=${DB_TABLE_NAME:-"SimpleSubscribe"}

echo "Creating DynamoDB Table..."
aws dynamodb create-table \
    --table-name ${BUILD_TABLE_NAME} \
    --attribute-definitions \
        AttributeName=email,AttributeType=S \
    --key-schema \
        AttributeName=email,KeyType=HASH \
    --provisioned-throughput ReadCapacityUnits=5,WriteCapacityUnits=5
