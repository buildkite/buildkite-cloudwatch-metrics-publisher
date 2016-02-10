package main

import (
	"log"

	"github.com/apex/go-apex"
	apexcfn "github.com/apex/go-apex/cloudformation"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchevents"
	"github.com/aws/aws-sdk-go/service/lambda"
)

func main() {
	sess := session.New()
	svc := cloudwatchevents.New(sess)
	lambdaSvc := lambda.New(sess)

	apexcfn.HandleFunc(func(req *apexcfn.Request, ctx *apex.Context) (interface{}, error) {
		switch req.RequestType {
		case "Create":
			ruleArn, err := createEventRule(req, svc)
			if err != nil {
				return nil, err
			}
			log.Printf("Created event rule %s", ruleArn)
			err = addLambdaPermission(req, ruleArn, lambdaSvc)
			if err != nil {
				return nil, err
			}
			err = createPutTargets(req, svc)
			if err != nil {
				return nil, err
			}
			return map[string]string{"RuleArn": ruleArn}, nil

		case "Delete":
			if err := deleteEventRule(req, svc); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
}

func createEventRule(req *apexcfn.Request, svc *cloudwatchevents.CloudWatchEvents) (string, error) {
	resp, err := svc.PutRule(&cloudwatchevents.PutRuleInput{
		Name:               aws.String(req.ResourceProperties["Name"].(string)),
		ScheduleExpression: aws.String(req.ResourceProperties["ScheduleExpression"].(string)),
	})
	if err != nil {
		return "", err
	}
	return *resp.RuleArn, nil
}

func deleteEventRule(req *apexcfn.Request, svc *cloudwatchevents.CloudWatchEvents) error {
	_, err := svc.DeleteRule(&cloudwatchevents.DeleteRuleInput{
		Name: aws.String(req.ResourceProperties["Name"].(string)),
	})
	if err != nil {
		return err
	}
	return nil
}

func addLambdaPermission(req *apexcfn.Request, srcArn string, svc *lambda.Lambda) error {
	_, err := svc.AddPermission(&lambda.AddPermissionInput{
		Action:       aws.String("lambda:InvokeFunction"),
		FunctionName: aws.String(req.ResourceProperties["LambdaArn"].(string)),
		Principal:    aws.String("events.amazonaws.com"),
		StatementId:  aws.String("lambda-permission-for-cloudwatch"),
		SourceArn:    aws.String(srcArn),
	})
	if err != nil {
		return err
	}
	return nil
}

func createPutTargets(req *apexcfn.Request, svc *cloudwatchevents.CloudWatchEvents) error {
	resp, err := svc.PutTargets(&cloudwatchevents.PutTargetsInput{
		Rule: aws.String(req.ResourceProperties["Name"].(string)),
		Targets: []*cloudwatchevents.Target{{
			Arn: aws.String(req.ResourceProperties["LambdaArn"].(string)),
			Id:  aws.String("1"),
		}},
	})

	log.Printf("%#v", resp)

	if err != nil {
		return err
	}

	return nil
}
