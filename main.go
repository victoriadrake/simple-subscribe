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
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	sestypes "github.com/aws/aws-sdk-go-v2/service/ses/types"
	"github.com/google/uuid"
)

// Define interfaces for AWS service clients to enable mocking.
// The concrete v2 clients (*dynamodb.Client, *ses.Client) satisfy these.
type DynamoDBAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

type SESAPI interface {
	SendEmail(ctx context.Context, params *ses.SendEmailInput, optFns ...func(*ses.Options)) (*ses.SendEmailOutput, error)
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
		Key: map[string]dynamodbtypes.AttributeValue{
			"email": &dynamodbtypes.AttributeValueMemberS{Value: email},
		},
		TableName: aws.String(table),
	}

	result, err := svc.GetItem(context.Background(), input)
	if err != nil {
		log.Print(err.Error())
		return false, err
	}
	if result.Item == nil {
		return false, nil
	}
	// Double check that the resulting email and id matches the input, return emailExistsWithId == true
	emailAttr, emailOk := result.Item["email"].(*dynamodbtypes.AttributeValueMemberS)
	idAttr, idOk := result.Item["id"].(*dynamodbtypes.AttributeValueMemberS)
	if emailOk && idOk && emailAttr.Value == email && idAttr.Value == id {
		return true, nil
	}
	log.Printf("No match for email: %s with id: %s", email, id)
	return false, nil
}

// Edits an existing email's attributes. No authorization is performed here, so ensure you check that values of email and id match before calling this function.
func updateItemInDynamoDB(svc DynamoDBAPI, email string, id string, timestamp string, confirm bool) (*dynamodb.UpdateItemOutput, error) {
	table := os.Getenv("DB_TABLE_NAME")

	input := &dynamodb.UpdateItemInput{
		// Provide the key to use for finding the right item.
		// Only matching on email means that a duplicate subscription request will override the first id.
		Key: map[string]dynamodbtypes.AttributeValue{
			"email": &dynamodbtypes.AttributeValueMemberS{Value: email},
		},
		// Give the keys to be updated a shorthand to reference
		ExpressionAttributeNames: map[string]string{
			"#ID": "id",
			"#T":  "timestamp",
			"#C":  "confirm",
		},
		// Give the incoming values a shorthand to reference
		ExpressionAttributeValues: map[string]dynamodbtypes.AttributeValue{
			":idval":      &dynamodbtypes.AttributeValueMemberS{Value: id},
			":timeval":    &dynamodbtypes.AttributeValueMemberS{Value: timestamp},
			":confirmval": &dynamodbtypes.AttributeValueMemberBOOL{Value: confirm},
		},
		// Use the shorthand references to update these keys
		UpdateExpression: aws.String("SET #C = :confirmval, #T = :timeval, #ID = :idval"),
		TableName:        aws.String(table),
	}

	result, err := svc.UpdateItem(context.Background(), input)
	if err != nil {
		log.Print(err.Error())
	}
	return result, err
}

// Delete an email from the table if the id matches.
func deleteEmailFromDynamoDb(svc DynamoDBAPI, email string, id string) (*dynamodb.DeleteItemOutput, error) {
	table := os.Getenv("DB_TABLE_NAME")
	input := &dynamodb.DeleteItemInput{
		Key: map[string]dynamodbtypes.AttributeValue{
			"email": &dynamodbtypes.AttributeValueMemberS{Value: email},
		},
		ExpressionAttributeValues: map[string]dynamodbtypes.AttributeValue{
			":emailval": &dynamodbtypes.AttributeValueMemberS{Value: email},
			":idval":    &dynamodbtypes.AttributeValueMemberS{Value: id},
		},
		// Find an item that matches both email and id
		ConditionExpression: aws.String("email = :emailval AND id = :idval"),
		TableName:           aws.String(table),
	}

	result, err := svc.DeleteItem(context.Background(), input)
	if err != nil {
		log.Println(err.Error())
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
		Destination: &sestypes.Destination{
			ToAddresses: []string{email},
		},
		Message: &sestypes.Message{
			Body: &sestypes.Body{
				Html: &sestypes.Content{
					Charset: aws.String("UTF-8"),
					Data:    aws.String(msg),
				},
				Text: &sestypes.Content{
					Charset: aws.String("UTF-8"),
					Data:    aws.String(txt),
				},
			},
			Subject: &sestypes.Content{
				Charset: aws.String("UTF-8"),
				Data:    aws.String("Confirm your subscription"),
			},
		},
		ReturnPath: aws.String(os.Getenv("SENDER_EMAIL")),
		Source:     aws.String(source),
	}

	result, err := sesSvc.SendEmail(context.Background(), input)
	if err != nil {
		log.Print(err.Error())
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
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("unable to load AWS SDK config: %s", err)
	}
	clients := &ServiceClients{
		DynamoDB: dynamodb.NewFromConfig(cfg),
		SES:      ses.NewFromConfig(cfg),
	}
	lambda.Start(func(ctx context.Context, event events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
		return lambdaHandler(ctx, clients, event)
	})
}
