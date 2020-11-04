package main

import (
	"context"
	"log"
	"time"

	"net/http"
	"net/mail"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/google/uuid"
)

const errorPage = "/error.html"
const successPage = "/success.html"

type searchResponse struct {
	Result       []searchResult `json:"result"`
	ContactCount int            `json:"contact_count"`
}

type searchResult struct {
	Id    string `json:"id"`
	Email string `json:"email"`
}

func addContactToDynamoDb(email string) {
	dynamoTableName := os.Getenv("DYNAMO_DB_TABLE_NAME")
	svc := dynamodb.New(session.New())

	input := &dynamodb.PutItemInput{
		Item: map[string]*dynamodb.AttributeValue{
			"email": {
				S: aws.String(email),
			},
			"id": {
				S: aws.String(uuid.New().String()),
			},
			"datetime_added": {
				S: aws.String(time.Now().Format("2006-01-02 15:04:05")),
			},
		},
		TableName: aws.String(dynamoTableName),
	}

	result, err := svc.PutItem(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case dynamodb.ErrCodeConditionalCheckFailedException:
				log.Println(dynamodb.ErrCodeConditionalCheckFailedException, aerr.Error())
			case dynamodb.ErrCodeProvisionedThroughputExceededException:
				log.Println(dynamodb.ErrCodeProvisionedThroughputExceededException, aerr.Error())
			case dynamodb.ErrCodeResourceNotFoundException:
				log.Println(dynamodb.ErrCodeResourceNotFoundException, aerr.Error())
			case dynamodb.ErrCodeItemCollectionSizeLimitExceededException:
				log.Println(dynamodb.ErrCodeItemCollectionSizeLimitExceededException, aerr.Error())
			case dynamodb.ErrCodeTransactionConflictException:
				log.Println(dynamodb.ErrCodeTransactionConflictException, aerr.Error())
			case dynamodb.ErrCodeRequestLimitExceeded:
				log.Println(dynamodb.ErrCodeRequestLimitExceeded, aerr.Error())
			case dynamodb.ErrCodeInternalServerError:
				log.Println(dynamodb.ErrCodeInternalServerError, aerr.Error())
			default:
				log.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Println(err.Error())
		}
		return
	}

	log.Println(result)
}

func deleteContactFromDynamoDb(id string) {
	dynamoTableName := os.Getenv("DYNAMO_DB_TABLE_NAME")
	svc := dynamodb.New(session.New())
	input := &dynamodb.DeleteItemInput{
		Key: map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String(id),
			},
		},
		TableName: aws.String(dynamoTableName),
	}

	result, err := svc.DeleteItem(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case dynamodb.ErrCodeConditionalCheckFailedException:
				log.Println(dynamodb.ErrCodeConditionalCheckFailedException, aerr.Error())
			case dynamodb.ErrCodeProvisionedThroughputExceededException:
				log.Println(dynamodb.ErrCodeProvisionedThroughputExceededException, aerr.Error())
			case dynamodb.ErrCodeResourceNotFoundException:
				log.Println(dynamodb.ErrCodeResourceNotFoundException, aerr.Error())
			case dynamodb.ErrCodeItemCollectionSizeLimitExceededException:
				log.Println(dynamodb.ErrCodeItemCollectionSizeLimitExceededException, aerr.Error())
			case dynamodb.ErrCodeTransactionConflictException:
				log.Println(dynamodb.ErrCodeTransactionConflictException, aerr.Error())
			case dynamodb.ErrCodeRequestLimitExceeded:
				log.Println(dynamodb.ErrCodeRequestLimitExceeded, aerr.Error())
			case dynamodb.ErrCodeInternalServerError:
				log.Println(dynamodb.ErrCodeInternalServerError, aerr.Error())
			default:
				log.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Println(err.Error())
		}
		return
	}

	log.Println(result)
}

func lambdaHandler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	resp := events.APIGatewayProxyResponse{Headers: make(map[string]string)}
	resp.Headers["Access-Control-Allow-Origin"] = "*"
	resp.StatusCode = http.StatusSeeOther

	r := http.Request{}
	r.Header = make(map[string][]string)
	for k, v := range request.Headers {
		if k == "content-type" || k == "Content-Type" {
			r.Header.Set(k, v)
		}
	}
	if request.Path == "/subscribe" {
		email, err := mail.ParseAddress(request.QueryStringParameters["email"])

		notbot := request.QueryStringParameters["notbot"]
		isbot := request.QueryStringParameters["isbot"]

		if err != nil || notbot != "true" || isbot != "" {
			log.Println(err, email.Address)
			resp.Headers["Location"] = errorPage
			return resp, nil
		}

		addContactToDynamoDb(email.Address)
		if err != nil {
			log.Println(err)
			resp.Headers["Location"] = errorPage
			return resp, nil
		}
		resp.Headers["Location"] = successPage
		return resp, nil

	}

	if request.Path == "/unsubscribe" {
		email := request.QueryStringParameters["id"]
		if email == "" {
			log.Println("email parameter not supplied")
			log.Println(request.QueryStringParameters)
			resp.Headers["Location"] = errorPage
			return resp, nil
		}
		deleteContactFromDynamoDb(email)
		resp.Headers["Location"] = successPage
		return resp, nil
	}

	resp.Headers["Location"] = errorPage
	return resp, nil
}

func main() {
	lambda.Start(lambdaHandler)
}
