package discovery

import "context"

type Advertiser interface {
	Advertise(ctx context.Context, code string) error
	Stop() error
}
