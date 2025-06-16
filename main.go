package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"net/http"
	"net/mail"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/google/uuid"
)

// Define interfaces for AWS service clients to enable mocking
type DynamoDBAPI interface {
	GetItemWithContext(aws.Context, *dynamodb.GetItemInput, ...request.Option) (*dynamodb.GetItemOutput, error)
	UpdateItemWithContext(aws.Context, *dynamodb.UpdateItemInput, ...request.Option) (*dynamodb.UpdateItemOutput, error)
	DeleteItemWithContext(aws.Context, *dynamodb.DeleteItemInput, ...request.Option) (*dynamodb.DeleteItemOutput, error)
}

type SESAPI interface {
	SendEmailWithContext(aws.Context, *ses.SendEmailInput, ...request.Option) (*ses.SendEmailOutput, error)
}

// ServiceClients holds the AWS service clients
type ServiceClients struct {
	DynamoDB DynamoDBAPI
	SES      SESAPI
}

// Determine if an email exists with the given id.
func emailExistsWithId(svc DynamoDBAPI, email string, id string) (bool, error) {
	table := os.Getenv("DB_TABLE_NAME")
	input := &dynamodb.GetItemInput{
		// Get an item that matches email
		Key: map[string]*dynamodb.AttributeValue{
			"email": {
				S: aws.String(email),
			},
		},
		TableName: aws.String(table),
	}

	result, err := svc.GetItemWithContext(context.Background(), input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case dynamodb.ErrCodeProvisionedThroughputExceededException:
				log.Print(dynamodb.ErrCodeProvisionedThroughputExceededException, aerr.Error())
			case dynamodb.ErrCodeResourceNotFoundException:
				log.Print(dynamodb.ErrCodeResourceNotFoundException, aerr.Error())
			case dynamodb.ErrCodeRequestLimitExceeded:
				log.Print(dynamodb.ErrCodeRequestLimitExceeded, aerr.Error())
			case dynamodb.ErrCodeInternalServerError:
				log.Print(dynamodb.ErrCodeInternalServerError, aerr.Error())
			default:
				log.Print(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Print(err.Error())
		}
	}
	if result.Item == nil {
		return false, err
	}
	// Double check that the resulting email and id matches the input, return emailExistsWithId == true
	if (*result.Item["email"].S == email) && (*result.Item["id"].S == id) {
		return true, nil
	}
	log.Printf("No match for email: %s with id: %s", email, id)
	return false, err
}

// Edits an existing email's attributes. No authorization is performed here, so ensure you check that values of email and id match before calling this function.
func updateItemInDynamoDB(svc DynamoDBAPI, email string, id string, timestamp string, confirm bool) (*dynamodb.UpdateItemOutput, error) {
	table := os.Getenv("DB_TABLE_NAME")

	input := &dynamodb.UpdateItemInput{
		// Provide the key to use for finding the right item.
		// Only matching on email means that a duplicate subscription request will override the first id.
		Key: map[string]*dynamodb.AttributeValue{
			"email": {
				S: aws.String(email),
			},
		},
		// Give the keys to be updated a shorthand to reference
		ExpressionAttributeNames: map[string]*string{
			"#ID": aws.String("id"),
			"#T":  aws.String("timestamp"),
			"#C":  aws.String("confirm"),
		},
		// Give the incoming values a shorthand to reference
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":idval": {
				S: aws.String(id),
			},
			":timeval": {
				S: aws.String(timestamp),
			},
			// Always override existing bool
			":confirmval": {
				BOOL: aws.Bool(confirm),
			},
		},
		// Use the shorthand references to update these keys
		UpdateExpression: aws.String("SET #C = :confirmval, #T = :timeval, #ID = :idval"),
		TableName:        aws.String(table),
	}

	result, err := svc.UpdateItemWithContext(context.Background(), input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case dynamodb.ErrCodeConditionalCheckFailedException:
				log.Print(dynamodb.ErrCodeConditionalCheckFailedException, aerr.Error())
			case dynamodb.ErrCodeProvisionedThroughputExceededException:
				log.Print(dynamodb.ErrCodeProvisionedThroughputExceededException, aerr.Error())
			case dynamodb.ErrCodeResourceNotFoundException:
				log.Print(dynamodb.ErrCodeResourceNotFoundException, aerr.Error())
			case dynamodb.ErrCodeItemCollectionSizeLimitExceededException:
				log.Print(dynamodb.ErrCodeItemCollectionSizeLimitExceededException, aerr.Error())
			case dynamodb.ErrCodeTransactionConflictException:
				log.Print(dynamodb.ErrCodeTransactionConflictException, aerr.Error())
			case dynamodb.ErrCodeRequestLimitExceeded:
				log.Print(dynamodb.ErrCodeRequestLimitExceeded, aerr.Error())
			case dynamodb.ErrCodeInternalServerError:
				log.Print(dynamodb.ErrCodeInternalServerError, aerr.Error())
			default:
				log.Print(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Print(err.Error())
		}
	}
	return result, err
}

// Delete an email from the table if the id matches.
func deleteEmailFromDynamoDb(svc DynamoDBAPI, email string, id string) (*dynamodb.DeleteItemOutput, error) {
	table := os.Getenv("DB_TABLE_NAME")
	input := &dynamodb.DeleteItemInput{
		Key: map[string]*dynamodb.AttributeValue{
			"email": {
				S: aws.String(email),
			},
		},
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":emailval": {
				S: aws.String(email),
			},
			":idval": {
				S: aws.String(id),
			},
		},
		// Find an item that matches both email and id
		ConditionExpression: aws.String("email = :emailval AND id = :idval"),
		TableName:           aws.String(table),
	}

	result, err := svc.DeleteItemWithContext(context.Background(), input)
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
			log.Print(err.Error())
		}
	}
	return result, err
}

// Send a confirmation email with a link to complete subscription.
func sendEmailWithSES(sesSvc SESAPI, email string, id string) (*ses.SendEmailOutput, error) {
	log.Print("EMAIL: ", email)

	// HTML format
	msg := fmt.Sprintf("<p>Hello! You're receiving this email because you requested a subscription to my list.</p><p>To complete your subscription, please click this link to finish signing up:</p><p><a class=\"ulink\" href=\"%s%s/?email=%s&id=%s\" target=\"_blank\">Confirm subscription</a>.</p><p>If you did not request this email, you can safely ignore it. Your email address has not yet been added to my list.</p>", os.Getenv("API_URL"), os.Getenv("VERIFY_PATH"), email, id)

	// Plain text format
	txt := fmt.Sprintf("Hello! You're receiving this email because you requested a subscription to my list.\n\nTo complete your subscription, please visit this link to finish signing up.\n\n%s%s/?email=%s&id=%s\n\nIf you did not request this email, you can safely ignore it. Your email address has not yet been added to my list.", os.Getenv("API_URL"), os.Getenv("VERIFY_PATH"), email, id)

	// Build the "from" value
	source := fmt.Sprintf("\"%s\" <%s>", os.Getenv("SENDER_NAME"), os.Getenv("SENDER_EMAIL"))

	input := &ses.SendEmailInput{
		Destination: &ses.Destination{
			ToAddresses: []*string{
				aws.String(email),
			},
		},
		Message: &ses.Message{
			Body: &ses.Body{
				Html: &ses.Content{
					Charset: aws.String("UTF-8"),
					Data:    aws.String(msg),
				},
				Text: &ses.Content{
					Charset: aws.String("UTF-8"),
					Data:    aws.String(txt),
				},
			},
			Subject: &ses.Content{
				Charset: aws.String("UTF-8"),
				Data:    aws.String("Confirm your subscription"),
			},
		},
		ReturnPath: aws.String(os.Getenv("SENDER_EMAIL")),
		Source:     aws.String(source),
	}

	result, err := sesSvc.SendEmailWithContext(context.Background(), input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case ses.ErrCodeMessageRejected:
				log.Print(ses.ErrCodeMessageRejected, aerr.Error())
			case ses.ErrCodeMailFromDomainNotVerifiedException:
				log.Print(ses.ErrCodeMailFromDomainNotVerifiedException, aerr.Error())
			case ses.ErrCodeConfigurationSetDoesNotExistException:
				log.Print(ses.ErrCodeConfigurationSetDoesNotExistException, aerr.Error())
			case ses.ErrCodeConfigurationSetSendingPausedException:
				log.Print(ses.ErrCodeConfigurationSetSendingPausedException, aerr.Error())
			case ses.ErrCodeAccountSendingPausedException:
				log.Print(ses.ErrCodeAccountSendingPausedException, aerr.Error())
			default:
				log.Print(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Print(err.Error())
		}
		return result, err
	}
	log.Print(result)
	return result, nil
}

func lambdaHandler(ctx context.Context, clients *ServiceClients, event events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {

	errorPage := fmt.Sprintf("%s%s", os.Getenv("BASE_URL"), os.Getenv("ERROR_PAGE"))
	successPage := fmt.Sprintf("%s%s", os.Getenv("BASE_URL"), os.Getenv("SUCCESS_PAGE"))
	confirmSubscribe := fmt.Sprintf("%s%s", os.Getenv("BASE_URL"), os.Getenv("CONFIRM_SUBSCRIBE_PAGE"))
	confirmUnsubscribe := fmt.Sprintf("%s%s", os.Getenv("BASE_URL"), os.Getenv("CONFIRM_UNSUBSCRIBE_PAGE"))
	resp := events.APIGatewayV2HTTPResponse{Headers: make(map[string]string)}
	resp.Headers["Access-Control-Allow-Origin"] = "*"
	resp.StatusCode = http.StatusSeeOther

	// Request a new subscription.
	if event.RawPath == fmt.Sprintf("/%s/", os.Getenv("SUBSCRIBE_PATH")) {
		// Parse email from query string
		email, err := mail.ParseAddress(event.QueryStringParameters["email"])
		if err != nil {
			log.Print("Could not get email: ", err)
			resp.Headers["Location"] = errorPage
			return resp, err
		}

		// Add requested email, new id, timestamp, and confirm == false to the table.
		id := uuid.New().String()
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		_, uerr := updateItemInDynamoDB(clients.DynamoDB, email.Address, id, timestamp, false)
		if uerr != nil {
			log.Print("Could not update database: ", uerr)
			resp.Headers["Location"] = errorPage
			return resp, uerr
		}

		// Send confirmation email.
		_, serr := sendEmailWithSES(clients.SES, email.Address, id)
		if serr != nil {
			log.Print("Could not send confirmation email: ", serr)
			resp.Headers["Location"] = errorPage
			return resp, serr
		}

		// Sends requester to the SUCCESS_PATH in all cases that do not result in an error.
		// This mitigates enumeration.
		resp.Headers["Location"] = confirmSubscribe
		return resp, nil

	}

	// Verify a subscription and add email to list.
	if event.RawPath == fmt.Sprintf("/%s/", os.Getenv("VERIFY_PATH")) {
		// Parse email and id from query string.
		email, emailpresent := event.QueryStringParameters["email"]
		id, idpresent := event.QueryStringParameters["id"]
		if (emailpresent == false) || (idpresent == false) {
			log.Printf("Missing parameters in query string: %s", event.RawQueryString)
			resp.Headers["Location"] = errorPage
			return resp, nil
		}

		// Query for matching item. Both email and id must match.
		match, err := emailExistsWithId(clients.DynamoDB, email, id)

		if match == true {
			// Set confirm == true and update timestamp for when they subscribed.
			timestamp := time.Now().Format("2006-01-02 15:04:05")
			_, uerr := updateItemInDynamoDB(clients.DynamoDB, email, id, timestamp, true)
			if uerr != nil {
				log.Printf("Could not update item in database: %s\n with query string: %s", uerr, event.RawQueryString)
				resp.Headers["Location"] = errorPage
				return resp, uerr
			}
			resp.Headers["Location"] = successPage
			return resp, nil
		}
		// If details don't match, return error.
		if err != nil {
			log.Printf("Received a bad confirmation request: %s", event.RawQueryString)
			resp.Headers["Location"] = errorPage
			return resp, err
		}
	}

	// Delete an item from the list. Both email and id must match.
	if event.RawPath == fmt.Sprintf("/%s/", os.Getenv("UNSUBSCRIBE_PATH")) {
		// Parse email and id from query string.
		email, emailpresent := event.QueryStringParameters["email"]
		id, idpresent := event.QueryStringParameters["id"]
		if (emailpresent == false) || (idpresent == false) {
			log.Printf("Missing parameters in query string: %s", event.RawQueryString)
			resp.Headers["Location"] = errorPage
			return resp, nil
		}
		// Try to find a match
		match, err := emailExistsWithId(clients.DynamoDB, email, id)
		if match == true {
			// There's a matching item, so try to delete it
			_, derr := deleteEmailFromDynamoDb(clients.DynamoDB, email, id)
			if derr == nil {
				resp.Headers["Location"] = confirmUnsubscribe
			} else {
				log.Printf("Could not delete item: %s", derr)
				resp.Headers["Location"] = errorPage
			}
			return resp, derr
		}
		// If details don't match, return error
		if (match == false) || (err != nil) {
			log.Printf("Received a bad deletion request with no match or an error. Error: %s\n Query string: %s", err, event.RawQueryString)
			resp.Headers["Location"] = errorPage
			return resp, err
		}
	}

	// No event.RawPath match
	log.Printf("No path match for path: %s", event.RawPath)
	resp.Headers["Location"] = errorPage
	return resp, nil
}

func main() {
	sess := session.New()
	clients := &ServiceClients{
		DynamoDB: dynamodb.New(sess),
		SES:      ses.New(sess),
	}
	lambda.Start(func(ctx context.Context, event events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
		return lambdaHandler(ctx, clients, event)
	})
}
