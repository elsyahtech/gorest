package orm

import "context"

func (orm *ORM) newContext() (context.Context, context.CancelFunc) {
	ctx := orm.Context
	if ctx == nil {
		ctx = context.Background()
	}

	if orm.DatabaseConfig.Timeout > 0 {
		if _, ok := ctx.Deadline(); !ok {
			return context.WithTimeout(ctx, orm.DatabaseConfig.Timeout)
		}
	}

	return ctx, func() {}
}
