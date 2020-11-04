# Simple Subscribe 💌

Collect emails with a subscription box you can add to any page.

If you're interested in managing your own mailing list or newsletter, you can use Simple Subscribe to collect email addresses. It uses an AWS Lambda to handle subscribe and unsubscribe requests, and stores email addresses in a DynamoDB table.

From there, use your own email solution to mail your recipients.

- [What this Does](#what-this-does)
- [How this Works](#how-this-works)
  - [Subscribing](#subscribing)
  - [Providing Unsubscribe Links](#providing-unsubscribe-links)
- [Requirements and Installation](#requirements-and-installation)
  - [Environment Variables for Lambda](#environment-variables-for-lambda)
  - [Create the Sign Up Form](#create-the-sign-up-form)
- [Security Considerations](#security-considerations)
  - [Time-Limited Tokens](#time-limited-tokens)
  - [Periodic Clean Up](#periodic-clean-up)
- [Disclaimer](#disclaimer)
- [Contributing](#contributing)
  - [Open an Issue](#open-an-issue)
  - [Send a Pull Request](#send-a-pull-request)

## What this Does

Simple Subscribe will let your visitors:

- Enter their email and hit a **Subscribe** button to sign up.
- Receive a confirmation email in their inbox to finish signing up.
- Send requests to unsubscribe from your list and automatically have their email removed.

To see what this API won't do, read [Disclaimer](#disclaimer).

## How this Works

### Subscribing

Simple Subscribe receives a GET request to your `SUBSCRIBE_PATH` with a query string containing the intended subscriber's email. It then generates an `id` value and adds both `email` and `id` to your DynamoDB table. The table item now looks like:

| email                    | confirm | id           | timestamp           |
| ------------------------ | ------- | ------------ | ------------------- |
| `subscriber@example.com` | _false_ | `uuid-xxxxx` | 2020-11-01 00:27:39 |

The intended subscriber then receives an email from SES containing a link. This link takes the format:

```url
<BASE_URL><VERIFY_PATH>/?email=subscriber@example.com&id=uuid-xxxxx
```

Visiting the link sends a request to your `VERIFY_PATH` with the `email` and `id`. Simple Subscribe ensures these values match the database values, then sets `confirm` to `true` and updates the timestamp. The table item now looks like:

| email                    | confirm | id           | timestamp           |
| ------------------------ | ------- | ------------ | ------------------- |
| `subscriber@example.com` | _true_  | `uuid-xxxxx` | 2020-11-01 00:37:39 |

When querying for people to send your newsletter, ensure you only return emails where `confirm` is `true`.

### Providing Unsubscribe Links

Simple Subscribe uses `email` and `id` as arguments to the function that deletes an item from your DynamoDB table. To allow people to remove themselves from your list, provide a URL in emails that includes their `email` and `id` as a query string in the `UNSUBSCRIBE_PATH`. It looks something like:

```url
<BASE_URL><UNSUBSCRIBE_PATH>/?email=subscriber@example.com&id=uuid-xxxxx
```

If the provided `email` and `id` match a database item, that item will be deleted.

## Requirements and Installation

You can create the following required resources via the AWS web console. See the provided links.

- [Create a DynamoDB table](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/getting-started-step-1.html) with key `email`, a string. Key names are case-sensitive.
- [Create a Lambda Function](https://docs.aws.amazon.com/lambda/latest/dg/getting-started-create-function.html) with `main.go` uploaded as the code, and appropriate environment variables (see below). Ensure it has [permissions](https://docs.aws.amazon.com/lambda/latest/dg/lambda-permissions.html) to access DynamoDB and SES.
- [Set up an API Gateway trigger](https://docs.aws.amazon.com/lambda/latest/dg/services-apigateway.html?icmpid=docs_lambda_console) for your Lambda. Ensure [`payloadFormatVersion` for your integration](https://docs.aws.amazon.com/apigateway/latest/developerguide/http-api-develop-integrations-lambda.html) is `2.0`.
- [Set up AWS Simple Email Service](https://docs.aws.amazon.com/ses/latest/DeveloperGuide/send-email-set-up.html) for sending a subscription confirmation email. If it's your first time sending with SES, you may need to [add and verify your email address or domain](https://docs.aws.amazon.com/ses/latest/DeveloperGuide/verify-addresses-and-domains.html).
- Optionally, if you're interested in giving your API a custom domain, see the [AWS docs on setting up custom domain names for APIs](https://docs.aws.amazon.com/apigateway/latest/developerguide/how-to-custom-domains.html).

The `scripts/` directory has some helpers in it. To use these:

1. [Install AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-install.html) and [set up credentials](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/serverless-getting-started-set-up-credentials.html) on your machine.
2. Create a `.env` in this repository's root with the appropriate values as described below.

### Environment Variables for Lambda

The API will look for the following environment variables:

- `DB_TABLE_NAME`: your DynamoDB table
- `BASE_URL`: the address of your site, beginning with `https://` and ending with `/`
- `API_URL`: the endpoint of your API, ending with `/`

As well as these website pages:

- `CONFIRM_SUBSCRIBE_PAGE`: the path of the page your subscriber sees after submitting their email, e.g. `confirm`
- `SUCCESS_PAGE`: the path of the page your subscriber sees when they complete sign up, e.g. `success`
- `ERROR_PAGE`: the path of your error page, e.g. `error`
- `CONFIRM_UNSUBSCRIBE_PAGE`: the path of the page shown after someone successfully unsubscribes, e.g. `unsubscribed`

As well as these API endpoints:

- `SUBSCRIBE_PATH`: the name of your subscription endpoint, e.g. `signup`
- `UNSUBSCRIBE_PATH`: the name of your unsubscribe endpoint, e.g. `unsubscribe`
- `VERIFY_PATH`: the name of your email verification endpoint, e.g. `verify`
- `SENDER_EMAIL`: the email your confirmation message will be coming from
- `SENDER_NAME`: the name you'd like the confirmation message to come from

Pages that your subscriber is sent to after an action are constructed with the base URL in the format `<BASE_URL><SUCCESS_PAGE>`.

You can [input Lambda environment variables in the AWS console](https://docs.aws.amazon.com/lambda/latest/dg/configuration-envvars.html), or use the AWS CLI.

If you're using the AWS CLI, you can pass the environment variables for Lambda in the following shorthand format:

```sh
Variables={KeyName1=string,KeyName2=string}
```

The script `update-lambda.sh` is provided for convenience. It will upload `main.go` to your Lambda function and replace Lambda environment variables for you by sourcing `.env`. Ensure that `LAMBDA_ENV` is present to hold them.

Here's an example of a suitable `.env` that you can copy and modify:

```text
NAME="simple-subscribe"
DB_TABLE_NAME="SimpleSubscribe"

LAMBDA_ENV="Variables={\
DB_TABLE_NAME=SimpleSubscribe,\
BASE_URL=https://example.com/,\
API_URL=https://api.example.com/,\
ERROR_PAGE=error,\
SUCCESS_PAGE=success,\
CONFIRM_SUBSCRIBE_PAGE=confirm,\
CONFIRM_UNSUBSCRIBE_PAGE=unsubscribed,\
SUBSCRIBE_PATH=signup,\
UNSUBSCRIBE_PATH=unsubscribe,\
VERIFY_PATH=verify,\
SENDER_EMAIL=no-reply@example.com,\
SENDER_NAME='Ford Prefect'}"
```

While none of these are private or secret, it's good practice to have Git ignore environment variables. You can do this with `echo .env >> .gitignore` if it's not already there.

### Create the Sign Up Form

Your visitors will need a form to put their email into. A basic form with minimal, modern styling is included at `docs/index.html`. You can copy and paste it into any HTML page.

## Security Considerations

Standard considerations apply:

- Principle of least privilege: ensure your people and functions have only the minimum necessary permissions for accessing each of your AWS resources.
- Encryption: ensure your website is using HTTPS in general, and in particular for requests sent to your API.
- Validation: ensure your subscription form only processes input in the form of valid email addresses. (This is mostly handled for you by the browser if you use `<input type="email" ...>`.)

Here are some additional security features you may consider:

### Time-Limited Tokens

The `id` in Simple Subscribe is a UUID that acts as a token to permit confirming or unsubscribing emails. You may wish to expire or rotate these tokens after a certain time frame. You can do this with a periodic clean up (below) or with an AWS Lambda that provides more nuanced timing. Ensure that expiring tokens does not prevent a subscriber from unsubscribing.

### Periodic Clean Up

It would be a good idea to periodically clean up your DynamoDB table to avoid retaining email addresses where `confirm` is `false` past a certain time frame.

If you are particularly concerned about data integrity, you may want to explore [On-Demand Backup](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/backuprestore_HowItWorks.html) or [Point-in-Time Recovery](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/PointInTimeRecovery.html) for DynamoDB.

## Disclaimer

This is a small-scope project that would comprise only part of a production-ready application. Here are a few things this project does not intend to do:

- Rate limit repeated subscription requests
- Any type of CAPTCHA or is-it-a-human verification prior to form submission
- Input validation on the email in the included sign up form, besides [what browsers provide](https://developer.mozilla.org/en-US/docs/Web/HTML/Element/input/email#Validation)
- Send newsletters to your list
- Dance the samba 💃

## Contributing

Simple Subscribe would be happy to have your contribution! Add helper scripts, improve the code, or even just fix a typo you found.

Here are a couple ways you can help out. Thank you for being a part of this open source project! 💕

### Open an Issue

Please open an issue to tell me about bugs, or anything that might need fixing or updating.

### Send a Pull Request

If you would like to change or fix something yourself, a pull request (PR) is most welcome! Please open an issue before you start working. That way, you can let other people know that you're taking care of it and no one ends up doing extra work.

Please [fork the repository](https://help.github.com/en/github/getting-started-with-github/fork-a-repo), then check out a local branch that includes the issue number, such as `fix-<issue number>`. For example, `git checkout -b fix-42`.

Before you submit your PR, make sure your fork is [synced with `master`](https://help.github.com/en/github/collaborating-with-issues-and-pull-requests/syncing-a-fork), then [create a PR](https://help.github.com/en/github/collaborating-with-issues-and-pull-requests/creating-a-pull-request-from-a-fork). You may want to [allow edits from maintainers](https://help.github.com/en/github/collaborating-with-issues-and-pull-requests/allowing-changes-to-a-pull-request-branch-created-from-a-fork) so that I can help with small changes like fixing typos.
