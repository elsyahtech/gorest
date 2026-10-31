package orm

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type structBuildMongoQuery struct {
	filter     bson.M
	structInfo *structInfo
	preload    *mongoPreloadPlan
	tableName  string
	sort       bson.D
	grouped    bool
	isSlice    bool
	joining    bool
}

func (orm *ORM) buildMongoQuery(req *structBuildMongoQuery) (pipeline mongo.Pipeline, findOpts *options.FindOptionsBuilder, err error) {
	switch {
	case orm.IsDistinct:
		pipeline, err = orm.buildMongoDistinctPipeline(req.filter, req.sort, req.tableName)

		return pipeline, nil, err
	case req.grouped:
		if req.isSlice {
			pipeline, err = orm.buildMongoGroupPipeline(req.tableName, req.filter, req.sort, req.structInfo.structType)
		} else {
			pipeline, err = orm.buildMongoSingleGroupPipeline(req.tableName, req.filter, req.sort, req.structInfo.structType)
		}

		return pipeline, nil, err
	case req.joining:
		pipeline, err = orm.buildMongoJoinPipeline(req.tableName, req.filter, req.sort, req.structInfo.structType)

		return pipeline, nil, err
	default:
		findOpts, err = orm.buildMongoOptionFindMongo(req.tableName, req.preload, req.sort, req.isSlice)

		return nil, findOpts, err
	}
}
