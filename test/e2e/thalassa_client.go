//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"

	"github.com/thalassa-cloud/client-go/objectstorage"
	thalassaclient "github.com/thalassa-cloud/client-go/pkg/client"
)

// NewE2EObjectStorageClient builds an objectstorage client from e2e env config.
func NewE2EObjectStorageClient(cfg ThalassaE2EConfig) (*objectstorage.Client, error) {
	token, err := ReadTokenFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	opts := []thalassaclient.Option{
		thalassaclient.WithBaseURL(cfg.ThalassaURL),
		thalassaclient.WithOrganisation(cfg.Organisation),
		thalassaclient.WithAuthPersonalToken(string(token)),
		thalassaclient.WithUserAgent("thalassa-objectstorage-manager-e2e"),
	}
	if cfg.Insecure {
		opts = append(opts, thalassaclient.WithInsecure())
	}
	raw, err := thalassaclient.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	return objectstorage.New(raw)
}

// AssertBucketReady verifies the Thalassa bucket exists and is ready-ish.
func AssertBucketReady(ctx context.Context, client *objectstorage.Client, bucketName string) error {
	b, err := client.GetBucket(ctx, bucketName)
	if err != nil {
		return err
	}
	if b == nil || b.Name == "" {
		return fmt.Errorf("bucket %q not found", bucketName)
	}
	status := strings.ToLower(strings.TrimSpace(b.Status))
	if status != "" && status != "ready" {
		return fmt.Errorf("bucket %q status %q", bucketName, b.Status)
	}
	return nil
}

// CloudResourceGone reports whether err indicates the remote resource was deleted.
func CloudResourceGone(err error) bool {
	return err != nil && thalassaclient.IsNotFound(err)
}
