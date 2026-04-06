package google

import (
	vision "cloud.google.com/go/vision/apiv1"
	"context"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/encrypt"
	"google.golang.org/api/option"
)

type Vision struct {
	*vision.ImageAnnotatorClient
}

type VisionConfig struct {
	CredentialsJson string
	ProjectId       string
}

func NewVision(ctx context.Context, cnf VisionConfig, opts ...option.ClientOption) (*Vision, error) {
	deData, err := encrypt.Base64Decode([]byte(cnf.CredentialsJson))
	if err != nil {
		return nil, err
	}
	opt := option.WithCredentialsJSON(deData)
	opts = append(opts, opt)
	client, err := vision.NewImageAnnotatorClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	v := &Vision{}
	v.ImageAnnotatorClient = client

	return v, nil
}
