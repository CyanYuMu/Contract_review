package google

import (
	"cloud.google.com/go/storage"
	"context"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/encrypt"
	"google.golang.org/api/option"
)

type Storage struct {
	name string
	*storage.BucketHandle
}

func (s *Storage) Name() string {
	return s.name
}

type ConfigGcs struct {
	CredentialsJson string
	Bucket          string
}

type GcPubSub struct {
	CredentialsJson string
	ProjectId       string
	Options         []option.ClientOption
}

func NewStorageClient(ctx context.Context, cnf ConfigGcs, opts ...option.ClientOption) (*Storage, error) {
	byteData, err := encrypt.Base64Decode([]byte(cnf.CredentialsJson))
	if err != nil {
		return nil, err
	}
	opts = append(opts, option.WithCredentialsJSON(byteData))
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, err
	}

	// 使用 GCS 客户端执行操作
	cli := &Storage{}

	cli.BucketHandle = client.Bucket(cnf.Bucket)
	cli.name = cnf.Bucket

	return cli, nil
}
