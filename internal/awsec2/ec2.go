// Package awsec2 wraps the small subset of the EC2 API we need:
// start/stop an instance, wait until it is running, and read its public IP.
//
// Credentials are resolved via the SDK's default chain (env, shared config,
// SSO, IAM role, etc.), so there is no need to hard-code access keys.
package awsec2

import (
	"context"
	"errors"
	"fmt"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type Client struct {
	ec2 *ec2.Client
}

// New builds an EC2 client for the given region (empty = SDK default) and
// optional profile.
func New(ctx context.Context, region, profile string) (*Client, error) {
	var opts []func(*awsconfig.LoadOptions) error
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	if profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(profile))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	return &Client{ec2: ec2.NewFromConfig(cfg)}, nil
}

// State is the normalized view of an EC2 instance used by the CLI.
type State struct {
	InstanceID string
	State      types.InstanceStateName
	PublicIP   string
}

// Describe returns the current state of an instance.
func (c *Client) Describe(ctx context.Context, id string) (State, error) {
	out, err := c.ec2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{id},
	})
	if err != nil {
		return State{}, fmt.Errorf("describe %s: %w", id, err)
	}
	if len(out.Reservations) == 0 || len(out.Reservations[0].Instances) == 0 {
		return State{}, fmt.Errorf("instance %s not found", id)
	}
	inst := out.Reservations[0].Instances[0]
	s := State{InstanceID: id}
	if inst.State != nil {
		s.State = inst.State.Name
	}
	if inst.PublicIpAddress != nil {
		s.PublicIP = *inst.PublicIpAddress
	}
	return s, nil
}

// Start kicks off the instance if it is not already running, then waits
// until it reaches the running state.
func (c *Client) Start(ctx context.Context, id string) error {
	_, err := c.ec2.StartInstances(ctx, &ec2.StartInstancesInput{
		InstanceIds: []string{id},
	})
	if err != nil {
		return fmt.Errorf("start %s: %w", id, err)
	}
	waiter := ec2.NewInstanceRunningWaiter(c.ec2)
	if err := waiter.Wait(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{id},
	}, 5*time.Minute); err != nil {
		return fmt.Errorf("wait for %s to run: %w", id, err)
	}
	return nil
}

// Stop stops the instance without waiting for it to fully stop.
func (c *Client) Stop(ctx context.Context, id string) error {
	_, err := c.ec2.StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: []string{id},
	})
	if err != nil {
		return fmt.Errorf("stop %s: %w", id, err)
	}
	return nil
}

// WaitForPublicIP polls Describe with exponential backoff until the instance
// reports a public IP or the context/timeout fires. Replaces the fixed
// sleep(10) in the Python original.
func (c *Client) WaitForPublicIP(ctx context.Context, id string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	delay := 500 * time.Millisecond
	for {
		s, err := c.Describe(ctx, id)
		if err != nil {
			return "", err
		}
		if s.PublicIP != "" {
			return s.PublicIP, nil
		}
		if time.Now().After(deadline) {
			return "", errors.New("timed out waiting for public IP")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(delay):
		}
		if delay < 5*time.Second {
			delay *= 2
		}
	}
}
