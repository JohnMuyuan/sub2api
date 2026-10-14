package service

import (
	"context"
)

type RegionalAccountGuard interface {
	RegionalAccountAllowed(context.Context, int64) error
	RegionalAccountReport(context.Context, int64, int) error
}
