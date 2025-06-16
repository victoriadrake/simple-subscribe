package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/aws/aws-sdk-go/service/ses/sesiface"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockDynamoDBClient is a mock implementation of dynamodbiface.DynamoDBAPI
type MockDynamoDBClient struct {
	dynamodbiface.DynamoDBAPI
	mock.Mock
}

func (m *MockDynamoDBClient) GetItemWithContext(ctx aws.Context, input *dynamodb.GetItemInput, opts ...request.Option) (*dynamodb.GetItemOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*dynamodb.GetItemOutput), args.Error(1)
}

func (m *MockDynamoDBClient) UpdateItemWithContext(ctx aws.Context, input *dynamodb.UpdateItemInput, opts ...request.Option) (*dynamodb.UpdateItemOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*dynamodb.UpdateItemOutput), args.Error(1)
}

func (m *MockDynamoDBClient) DeleteItemWithContext(ctx aws.Context, input *dynamodb.DeleteItemInput, opts ...request.Option) (*dynamodb.DeleteItemOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*dynamodb.DeleteItemOutput), args.Error(1)
}

// MockSESClient is a mock implementation of sesiface.SESAPI
type MockSESClient struct {
	sesiface.SESAPI
	mock.Mock
}

func (m *MockSESClient) SendEmailWithContext(ctx aws.Context, input *ses.SendEmailInput, opts ...request.Option) (*ses.SendEmailOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*ses.SendEmailOutput), args.Error(1)
}

func TestEmailExistsWithId(t *testing.T) {
	os.Setenv("DB_TABLE_NAME", "TestTable")

	tests := []struct {
		name          string
		email         string
		id            string
		mockGetItem   *dynamodb.GetItemOutput
		mockGetItemErr error
		expectedExist bool
		expectedErr   error
	}{
		{
			name:  "Email and ID match",
			email: "test@example.com",
			id:    "123",
			mockGetItem: &dynamodb.GetItemOutput{
				Item: map[string]*dynamodb.AttributeValue{
					"email": {S: aws.String("test@example.com")},
					"id":    {S: aws.String("123")},
				},
			},
			mockGetItemErr: nil,
			expectedExist:  true,
			expectedErr:    nil,
		},
		{
			name:  "Email exists, ID does not match",
			email: "test@example.com",
			id:    "456",
			mockGetItem: &dynamodb.GetItemOutput{
				Item: map[string]*dynamodb.AttributeValue{
					"email": {S: aws.String("test@example.com")},
					"id":    {S: aws.String("123")},
				},
			},
			mockGetItemErr: nil,
			expectedExist:  false,
			expectedErr:    nil,
		},
		{
			name:           "Email does not exist",
			email:          "nonexistent@example.com",
			id:             "789",
			mockGetItem:    &dynamodb.GetItemOutput{Item: nil},
			mockGetItemErr: nil,
			expectedExist:  false,
			expectedErr:    nil,
		},
		{
			name:           "DynamoDB error",
			email:          "error@example.com",
			id:             "abc",
			mockGetItem:    &dynamodb.GetItemOutput{},
			mockGetItemErr: errors.New("DynamoDB error"),
			expectedExist:  false,
			expectedErr:    errors.New("DynamoDB error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSvc := new(MockDynamoDBClient)
			mockSvc.On("GetItemWithContext", mock.Anything, mock.AnythingOfType("*dynamodb.GetItemInput")).Return(tt.mockGetItem, tt.mockGetItemErr)

			exists, err := emailExistsWithId(mockSvc, tt.email, tt.id)

			assert.Equal(t, tt.expectedExist, exists)
			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr.Error())
			} else {
				assert.NoError(t, err)
			}
			mockSvc.AssertExpectations(t)
		})
	}
}

func TestUpdateItemInDynamoDB(t *testing.T) {
	os.Setenv("DB_TABLE_NAME", "TestTable")

	tests := []struct {
		name            string
		email           string
		id              string
		timestamp       string
		confirm         bool
		mockUpdateItem  *dynamodb.UpdateItemOutput
		mockUpdateItemErr error
		expectedErr     error
	}{
		{
			name:            "Successful update",
			email:           "test@example.com",
			id:              "123",
			timestamp:       "2023-01-01 12:00:00",
			confirm:         true,
			mockUpdateItem:  &dynamodb.UpdateItemOutput{},
			mockUpdateItemErr: nil,
			expectedErr:     nil,
		},
		{
			name:            "DynamoDB error during update",
			email:           "error@example.com",
			id:              "456",
			timestamp:       "2023-01-01 12:00:00",
			confirm:         false,
			mockUpdateItem:  &dynamodb.UpdateItemOutput{},
			mockUpdateItemErr: errors.New("DynamoDB update error"),
			expectedErr:     errors.New("DynamoDB update error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSvc := new(MockDynamoDBClient)
			mockSvc.On("UpdateItemWithContext", mock.Anything, mock.AnythingOfType("*dynamodb.UpdateItemInput")).Return(tt.mockUpdateItem, tt.mockUpdateItemErr)

			_, err := updateItemInDynamoDB(mockSvc, tt.email, tt.id, tt.timestamp, tt.confirm)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr.Error())
			} else {
				assert.NoError(t, err)
			}
			mockSvc.AssertExpectations(t)
		})
	}
}

func TestDeleteEmailFromDynamoDb(t *testing.T) {
	os.Setenv("DB_TABLE_NAME", "TestTable")

	tests := []struct {
		name            string
		email           string
		id              string
		mockDeleteItem  *dynamodb.DeleteItemOutput
		mockDeleteItemErr error
		expectedErr     error
	}{
		{
			name:            "Successful deletion",
			email:           "test@example.com",
			id:              "123",
			mockDeleteItem:  &dynamodb.DeleteItemOutput{},
			mockDeleteItemErr: nil,
			expectedErr:     nil,
		},
		{
			name:            "ConditionalCheckFailedException (ID mismatch)",
			email:           "test@example.com",
			id:              "456",
			mockDeleteItem:  &dynamodb.DeleteItemOutput{},
			mockDeleteItemErr: errors.New("ConditionalCheckFailedException"),
			expectedErr:     errors.New("ConditionalCheckFailedException"),
		},
		{
			name:            "DynamoDB error during deletion",
			email:           "error@example.com",
			id:              "789",
			mockDeleteItem:  &dynamodb.DeleteItemOutput{},
			mockDeleteItemErr: errors.New("DynamoDB delete error"),
			expectedErr:     errors.New("DynamoDB delete error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSvc := new(MockDynamoDBClient)
			mockSvc.On("DeleteItemWithContext", mock.Anything, mock.AnythingOfType("*dynamodb.DeleteItemInput")).Return(tt.mockDeleteItem, tt.mockDeleteItemErr)

			_, err := deleteEmailFromDynamoDb(mockSvc, tt.email, tt.id)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr.Error())
			} else {
				assert.NoError(t, err)
			}
			mockSvc.AssertExpectations(t)
		})
	}
}

func TestSendEmailWithSES(t *testing.T) {
	os.Setenv("SENDER_NAME", "Test Sender")
	os.Setenv("SENDER_EMAIL", "sender@example.com")
	os.Setenv("API_URL", "https://api.example.com")
	os.Setenv("VERIFY_PATH", "/verify")

	tests := []struct {
		name          string
		email         string
		id            string
		mockSendEmail *ses.SendEmailOutput
		mockSendEmailErr error
		expectedErr   error
	}{
		{
			name:          "Successful email send",
			email:         "recipient@example.com",
			id:            "123",
			mockSendEmail: &ses.SendEmailOutput{},
			mockSendEmailErr: nil,
			expectedErr:   nil,
		},
		{
			name:          "SES error during send",
			email:         "error@example.com",
			id:            "456",
			mockSendEmail: &ses.SendEmailOutput{},
			mockSendEmailErr: errors.New("SES send error"),
			expectedErr:   errors.New("SES send error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSvc := new(MockSESClient)
			mockSvc.On("SendEmailWithContext", mock.Anything, mock.AnythingOfType("*ses.SendEmailInput")).Return(tt.mockSendEmail, tt.mockSendEmailErr)

			_, err := sendEmailWithSES(mockSvc, tt.email, tt.id)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr.Error())
			} else {
				assert.NoError(t, err)
			}
			mockSvc.AssertExpectations(t)
		})
	}
}

func TestLambdaHandler(t *testing.T) {
	// Set up environment variables for the handler
	os.Setenv("BASE_URL", "https://example.com")
	os.Setenv("ERROR_PAGE", "/error")
	os.Setenv("SUCCESS_PAGE", "/success")
	os.Setenv("CONFIRM_SUBSCRIBE_PAGE", "/confirm-subscribe")
	os.Setenv("CONFIRM_UNSUBSCRIBE_PAGE", "/confirm-unsubscribe")
	os.Setenv("SUBSCRIBE_PATH", "subscribe")
	os.Setenv("VERIFY_PATH", "verify")
	os.Setenv("UNSUBSCRIBE_PATH", "unsubscribe")
	os.Setenv("DB_TABLE_NAME", "TestTable")
	os.Setenv("SENDER_NAME", "Test Sender")
	os.Setenv("SENDER_EMAIL", "sender@example.com")
	os.Setenv("API_URL", "https://api.example.com")

	mockDynamoDB := new(MockDynamoDBClient)
	mockSES := new(MockSESClient)

	clients := &ServiceClients{
		DynamoDB: mockDynamoDB,
		SES:      mockSES,
	}

	tests := []struct {
		name           string
		event          events.APIGatewayV2HTTPRequest
		setupMocks     func()
		expectedStatus int
		expectedLocation string
		expectedErr      error // Added this field
	}{
		{
			name: "Subscribe - Success",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/subscribe/",
				QueryStringParameters: map[string]string{
					"email": "new@example.com",
				},
			},
			setupMocks: func() {
				mockDynamoDB.On("UpdateItemWithContext", mock.Anything, mock.AnythingOfType("*dynamodb.UpdateItemInput")).Return(&dynamodb.UpdateItemOutput{}, nil).Once()
				mockSES.On("SendEmailWithContext", mock.Anything, mock.AnythingOfType("*ses.SendEmailInput")).Return(&ses.SendEmailOutput{}, nil).Once()
			},
			expectedStatus: http.StatusSeeOther,
			expectedLocation: "https://example.com/confirm-subscribe",
			expectedErr:    nil,
		},
		{
			name: "Subscribe - Invalid Email",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/subscribe/",
				QueryStringParameters: map[string]string{
					"email": "invalid-email",
				},
			},
			setupMocks: func() {
				// No DynamoDB or SES calls expected
			},
			expectedStatus: http.StatusSeeOther,
			expectedLocation: "https://example.com/error",
			expectedErr:    errors.New("mail: missing '@' or angle-addr"),
		},
		{
			name: "Subscribe - DynamoDB Update Error",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/subscribe/",
				QueryStringParameters: map[string]string{
					"email": "db-error@example.com",
				},
			},
			setupMocks: func() {
				mockDynamoDB.On("UpdateItemWithContext", mock.Anything, mock.AnythingOfType("*dynamodb.UpdateItemInput")).Return(&dynamodb.UpdateItemOutput{}, errors.New("db error")).Once()
			},
			expectedStatus: http.StatusSeeOther,
			expectedLocation: "https://example.com/error",
			expectedErr:    errors.New("db error"),
		},
		{
			name: "Subscribe - SES Send Error",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/subscribe/",
				QueryStringParameters: map[string]string{
					"email": "ses-error@example.com",
				},
			},
			setupMocks: func() {
				mockDynamoDB.On("UpdateItemWithContext", mock.Anything, mock.AnythingOfType("*dynamodb.UpdateItemInput")).Return(&dynamodb.UpdateItemOutput{}, nil).Once()
				mockSES.On("SendEmailWithContext", mock.Anything, mock.AnythingOfType("*ses.SendEmailInput")).Return(&ses.SendEmailOutput{}, errors.New("ses error")).Once()
			},
			expectedStatus: http.StatusSeeOther,
			expectedLocation: "https://example.com/error",
			expectedErr:    errors.New("ses error"),
		},
		{
			name: "Verify - Success",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/verify/",
				QueryStringParameters: map[string]string{
					"email": "existing@example.com",
					"id":    "existing-id",
				},
			},
			setupMocks: func() {
				mockDynamoDB.On("GetItemWithContext", mock.Anything, mock.AnythingOfType("*dynamodb.GetItemInput")).Return(&dynamodb.GetItemOutput{
					Item: map[string]*dynamodb.AttributeValue{
						"email": {S: aws.String("existing@example.com")},
						"id":    {S: aws.String("existing-id")},
					},
				}, nil).Once()
				mockDynamoDB.On("UpdateItemWithContext", mock.Anything, mock.AnythingOfType("*dynamodb.UpdateItemInput")).Return(&dynamodb.UpdateItemOutput{}, nil).Once()
			},
			expectedStatus: http.StatusSeeOther,
			expectedLocation: "https://example.com/success",
			expectedErr:    nil,
		},
		{
			name: "Verify - Missing Parameters",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/verify/",
				QueryStringParameters: map[string]string{
					"email": "missing@example.com",
				},
			},
			setupMocks: func() {
				// No DynamoDB calls expected
			},
			expectedStatus: http.StatusSeeOther,
			expectedLocation: "https://example.com/error",
			expectedErr:    nil, // lambdaHandler returns nil error for missing params in this case
		},
		{
			name: "Verify - ID Mismatch",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/verify/",
				QueryStringParameters: map[string]string{
					"email": "mismatch@example.com",
					"id":    "wrong-id",
				},
			},
			setupMocks: func() {
				mockDynamoDB.On("GetItemWithContext", mock.Anything, mock.AnythingOfType("*dynamodb.GetItemInput")).Return(&dynamodb.GetItemOutput{
					Item: map[string]*dynamodb.AttributeValue{
						"email": {S: aws.String("mismatch@example.com")},
						"id":    {S: aws.String("correct-id")},
					},
				}, nil).Once()
			},
			expectedStatus: http.StatusSeeOther,
			expectedLocation: "https://example.com/error",
			expectedErr:    nil, // emailExistsWithId returns nil error if no match, lambdaHandler returns nil
		},
		{
			name: "Verify - DynamoDB Get Error",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/verify/",
				QueryStringParameters: map[string]string{
					"email": "get-error@example.com",
					"id":    "some-id",
				},
			},
			setupMocks: func() {
				mockDynamoDB.On("GetItemWithContext", mock.Anything, mock.AnythingOfType("*dynamodb.GetItemInput")).Return(&dynamodb.GetItemOutput{}, errors.New("get error")).Once()
			},
			expectedStatus: http.StatusSeeOther,
			expectedLocation: "https://example.com/error",
			expectedErr:    errors.New("get error"),
		},
		{
			name: "Verify - DynamoDB Update Error",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/verify/",
				QueryStringParameters: map[string]string{
					"email": "update-error@example.com",
					"id":    "some-id",
				},
			},
			setupMocks: func() {
				mockDynamoDB.On("GetItemWithContext", mock.Anything, mock.AnythingOfType("*dynamodb.GetItemInput")).Return(&dynamodb.GetItemOutput{
					Item: map[string]*dynamodb.AttributeValue{
						"email": {S: aws.String("update-error@example.com")},
						"id":    {S: aws.String("some-id")},
					},
				}, nil).Once()
				mockDynamoDB.On("UpdateItemWithContext", mock.Anything, mock.AnythingOfType("*dynamodb.UpdateItemInput")).Return(&dynamodb.UpdateItemOutput{}, errors.New("update error")).Once()
			},
			expectedStatus: http.StatusSeeOther,
			expectedLocation: "https://example.com/error",
			expectedErr:    errors.New("update error"),
		},
		{
			name: "Unsubscribe - Success",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/unsubscribe/",
				QueryStringParameters: map[string]string{
					"email": "unsub@example.com",
					"id":    "unsub-id",
				},
			},
			setupMocks: func() {
				mockDynamoDB.On("GetItemWithContext", mock.Anything, mock.AnythingOfType("*dynamodb.GetItemInput")).Return(&dynamodb.GetItemOutput{
					Item: map[string]*dynamodb.AttributeValue{
						"email": {S: aws.String("unsub@example.com")},
						"id":    {S: aws.String("unsub-id")},
					},
				}, nil).Once()
				mockDynamoDB.On("DeleteItemWithContext", mock.Anything, mock.AnythingOfType("*dynamodb.DeleteItemInput")).Return(&dynamodb.DeleteItemOutput{}, nil).Once()
			},
			expectedStatus: http.StatusSeeOther,
			expectedLocation: "https://example.com/confirm-unsubscribe",
			expectedErr:    nil,
		},
		{
			name: "Unsubscribe - Missing Parameters",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/unsubscribe/",
				QueryStringParameters: map[string]string{
					"email": "missing-unsub@example.com",
				},
			},
			setupMocks: func() {
				// No DynamoDB calls expected
			},
			expectedStatus: http.StatusSeeOther,
			expectedLocation: "https://example.com/error",
			expectedErr:    nil, // lambdaHandler returns nil error for missing params in this case
		},
		{
			name: "Unsubscribe - ID Mismatch (GetItem finds, DeleteItem fails)",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/unsubscribe/",
				QueryStringParameters: map[string]string{
					"email": "unsub-mismatch@example.com",
					"id":    "wrong-id",
				},
			},
			setupMocks: func() {
				mockDynamoDB.On("GetItemWithContext", mock.Anything, mock.AnythingOfType("*dynamodb.GetItemInput")).Return(&dynamodb.GetItemOutput{
					Item: map[string]*dynamodb.AttributeValue{
						"email": {S: aws.String("unsub-mismatch@example.com")},
						"id":    {S: aws.String("correct-id")},
					},
				}, nil).Once()
			},
			expectedStatus: http.StatusSeeOther,
			expectedLocation: "https://example.com/error",
			expectedErr:    nil, // emailExistsWithId returns nil error if no match, lambdaHandler returns nil
		},
		{
			name: "Unsubscribe - DynamoDB Get Error",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/unsubscribe/",
				QueryStringParameters: map[string]string{
					"email": "unsub-get-error@example.com",
					"id":    "some-id",
				},
			},
			setupMocks: func() {
				mockDynamoDB.On("GetItemWithContext", mock.Anything, mock.AnythingOfType("*dynamodb.GetItemInput")).Return(&dynamodb.GetItemOutput{}, errors.New("get error")).Once()
			},
			expectedStatus: http.StatusSeeOther,
			expectedLocation: "https://example.com/error",
			expectedErr:    errors.New("get error"),
		},
		{
			name: "Unsubscribe - DynamoDB Delete Error",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/unsubscribe/",
				QueryStringParameters: map[string]string{
					"email": "unsub-delete-error@example.com",
					"id":    "some-id",
				},
			},
			setupMocks: func() {
				mockDynamoDB.On("GetItemWithContext", mock.Anything, mock.AnythingOfType("*dynamodb.GetItemInput")).Return(&dynamodb.GetItemOutput{
					Item: map[string]*dynamodb.AttributeValue{
						"email": {S: aws.String("unsub-delete-error@example.com")},
						"id":    {S: aws.String("some-id")},
					},
				}, nil).Once()
				mockDynamoDB.On("DeleteItemWithContext", mock.Anything, mock.AnythingOfType("*dynamodb.DeleteItemInput")).Return(&dynamodb.DeleteItemOutput{}, errors.New("delete error")).Once()
			},
			expectedStatus: http.StatusSeeOther,
			expectedLocation: "https://example.com/error",
			expectedErr:    errors.New("delete error"),
		},
		{
			name: "Unknown Path",
			event: events.APIGatewayV2HTTPRequest{
				RawPath: "/unknown/",
			},
			setupMocks: func() {
				// No DynamoDB or SES calls expected
			},
			expectedStatus: http.StatusSeeOther,
			expectedLocation: "https://example.com/error",
			expectedErr:    nil, // lambdaHandler returns nil error for unknown path in this case
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks before each test
			mockDynamoDB.Calls = []mock.Call{}
			mockSES.Calls = []mock.Call{}

			tt.setupMocks()

			resp, err := lambdaHandler(context.Background(), clients, tt.event)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
			assert.Equal(t, tt.expectedLocation, resp.Headers["Location"])

			mockDynamoDB.AssertExpectations(t)
			mockSES.AssertExpectations(t)
		})
	}
}
