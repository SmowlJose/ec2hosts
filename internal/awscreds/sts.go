package awscreds

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// CallerIdentity is the relevant subset of STS:GetCallerIdentity: it
// tells the UI which account/principal the credentials belong to, which
// is the fastest feedback we can give that "yes, these creds work".
type CallerIdentity struct {
	Account string
	ARN     string
	UserID  string
}

// TestStatic resolves credentials from explicit access key / secret /
// optional session token (i.e. what the user just typed in the dialog)
// and runs GetCallerIdentity. It does NOT touch any file — it is safe
// to call before offering to save.
//
// A sensible ctx timeout is the caller's job; we also cap ourselves at
// 10 s so a stuck network doesn't make the UI feel frozen.
func TestStatic(ctx context.Context, region, accessKey, secret, sessionToken string) (CallerIdentity, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	provider := credentials.NewStaticCredentialsProvider(accessKey, secret, sessionToken)
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithCredentialsProvider(provider),
	}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return CallerIdentity{}, fmt.Errorf("load aws config: %w", err)
	}
	return callerIdentity(ctx, cfg)
}

// TestProfile resolves credentials from the on-disk profile (what the
// SDK would use at runtime) and runs GetCallerIdentity. Used for the
// "re-check" button after save, and for status detection on dialog
// open.
func TestProfile(ctx context.Context, profile, region string) (CallerIdentity, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var opts []func(*awsconfig.LoadOptions) error
	if profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(profile))
	}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return CallerIdentity{}, fmt.Errorf("load aws config: %w", err)
	}
	return callerIdentity(ctx, cfg)
}

func callerIdentity(ctx context.Context, cfg aws.Config) (CallerIdentity, error) {
	c := sts.NewFromConfig(cfg)
	out, err := c.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return CallerIdentity{}, err
	}
	id := CallerIdentity{}
	if out.Account != nil {
		id.Account = *out.Account
	}
	if out.Arn != nil {
		id.ARN = *out.Arn
	}
	if out.UserId != nil {
		id.UserID = *out.UserId
	}
	return id, nil
}
