package sns

import (
	"context"
	"github.com/applike/gosoline/pkg/cfg"
	"github.com/applike/gosoline/pkg/mon"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sns/snsiface"
)

//go:generate mockery -name Topic
type Topic interface {
	Publish(ctx context.Context, msg *string) error
}

type Settings struct {
	cfg.AppId
	TopicId string

	Arn string
}

type topic struct {
	logger mon.Logger
	client snsiface.SNSAPI

	settings Settings
}

func NewTopic(config cfg.Config, logger mon.Logger, s Settings) *topic {
	s.PadFromConfig(config)

	client := GetClient(config, logger)
	arn, err := CreateTopic(logger, client, s)

	if err != nil {
		panic(err)
	}

	s.Arn = arn

	return NewTopicWithInterfaces(logger, client, s)
}

func NewTopicWithInterfaces(logger mon.Logger, client snsiface.SNSAPI, s Settings) *topic {
	return &topic{
		logger:   logger,
		client:   client,
		settings: s,
	}
}

func (t *topic) Publish(ctx context.Context, msg *string) error {
	input := &sns.PublishInput{
		TopicArn: aws.String(t.settings.Arn),
		Message:  msg,
	}

	_, err := t.client.Publish(input)

	if err != nil {
		t.logger.WithFields(mon.Fields{
			"arn": t.settings.Arn,
		}).Error(err, "could not publish message to topic")
	}

	return err
}

func (t *topic) SubscribeSqs(queueArn string) error {
	exists, err := t.subscriptionExists(queueArn)

	if err != nil {
		t.logger.WithFields(mon.Fields{
			"topicArn": t.settings.Arn,
			"queueArn": queueArn,
		}).Error(err, "can not check if subscription already exists")

		return err
	}

	if exists {
		t.logger.WithFields(mon.Fields{
			"topicArn": t.settings.Arn,
			"queueArn": queueArn,
		}).Info("already subscribed to sns topic")

		return nil
	}

	input := &sns.SubscribeInput{
		TopicArn: aws.String(t.settings.Arn),
		Endpoint: aws.String(queueArn),
		Protocol: aws.String("sqs"),
	}

	_, err = t.client.Subscribe(input)

	if err != nil {
		t.logger.WithFields(mon.Fields{
			"topicArn": t.settings.Arn,
			"queueArn": queueArn,
		}).Error(err, "could not subscribe for sqs queue")
	}

	t.logger.WithFields(mon.Fields{
		"topicArn": t.settings.Arn,
		"queueArn": queueArn,
	}).Info("successful subscribed to sns topic")

	return err
}

func (t *topic) subscriptionExists(queueArn string) (bool, error) {
	subscriptions, err := t.listSubscriptions()

	if err != nil {
		return false, err
	}

	for _, s := range subscriptions {
		if *s.Endpoint == queueArn {
			return true, nil
		}
	}

	return false, nil
}

func (t *topic) listSubscriptions() ([]*sns.Subscription, error) {
	subscriptions := make([]*sns.Subscription, 0)

	input := &sns.ListSubscriptionsByTopicInput{
		TopicArn: aws.String(t.settings.Arn),
	}

	for {
		out, err := t.client.ListSubscriptionsByTopic(input)

		if err != nil {
			return nil, err
		}

		subscriptions = append(subscriptions, out.Subscriptions...)

		if out.NextToken == nil {
			break
		}

		input.NextToken = out.NextToken
	}

	return subscriptions, nil
}
