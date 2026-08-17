package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hydronica/ai-toolkit/cmd/db-query/internal/config"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var forbiddenMongoStages = map[string]struct{}{
	"$out":   {},
	"$merge": {},
}

type mongoRunner struct {
	client *mongo.Client
	dbName string
}

type mongoQuery struct {
	Collection string   `json:"collection"`
	Filter     bson.M   `json:"filter"`
	Projection bson.M   `json:"projection"`
	Sort       bson.D   `json:"sort"`
	Pipeline   []bson.M `json:"pipeline"`
	Limit      int      `json:"limit"`
}

func connectMongo(ctx context.Context, cfg *config.MongoConfig) (*mongoRunner, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.URI))
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, fmt.Errorf("ping: %w", err)
	}

	return &mongoRunner{
		client: client,
		dbName: cfg.DBName,
	}, nil
}

func (r *mongoRunner) Ping(ctx context.Context) error {
	return r.client.Ping(ctx, nil)
}

func (r *mongoRunner) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	disconnectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return r.client.Disconnect(disconnectCtx)
}

func (r *mongoRunner) ListSchema(ctx context.Context, limit int) (QueryOutput, error) {
	cursor, err := r.client.Database(r.dbName).ListCollections(ctx, bson.M{})
	if err != nil {
		return QueryOutput{}, fmt.Errorf("list schema: %w", err)
	}
	defer cursor.Close(ctx)

	type spec struct {
		Name string `bson:"name"`
		Type string `bson:"type"`
	}
	var specs []spec
	for cursor.Next(ctx) {
		var s spec
		if err := cursor.Decode(&s); err != nil {
			return QueryOutput{}, fmt.Errorf("list schema: %w", err)
		}
		specs = append(specs, s)
	}
	if err := cursor.Err(); err != nil {
		return QueryOutput{}, fmt.Errorf("list schema: %w", err)
	}

	sort.Slice(specs, func(i, j int) bool {
		return specs[i].Name < specs[j].Name
	})

	b := newCatalogBuilder(limit)
	for _, s := range specs {
		kind := strings.ToLower(strings.TrimSpace(s.Type))
		if kind == "" {
			kind = "collection"
		}
		if !b.add(s.Name, kind, "", "") {
			break
		}
	}
	return b.result(), nil
}

func (r *mongoRunner) RunQuery(ctx context.Context, query string, limit int) (QueryOutput, error) {
	mq, err := parseMongoQuery(query)
	if err != nil {
		return QueryOutput{}, err
	}
	if err := validateMongoQuery(mq); err != nil {
		return QueryOutput{}, err
	}

	queryLimit := limit
	if mq.Limit > 0 && (queryLimit == 0 || mq.Limit < queryLimit) {
		queryLimit = mq.Limit
	}

	collection := r.client.Database(r.dbName).Collection(mq.Collection)
	if len(mq.Pipeline) > 0 {
		return r.runAggregate(ctx, collection, mq, queryLimit)
	}
	return r.runFind(ctx, collection, mq, queryLimit)
}

func parseMongoQuery(query string) (*mongoQuery, error) {
	mq := &mongoQuery{}
	if err := json.Unmarshal([]byte(query), mq); err != nil {
		return nil, fmt.Errorf("mongo query must be JSON: %w", err)
	}
	if strings.TrimSpace(mq.Collection) == "" {
		return nil, errors.New("mongo query requires collection")
	}
	return mq, nil
}

func validateMongoQuery(mq *mongoQuery) error {
	for _, stage := range mq.Pipeline {
		for key := range stage {
			if _, forbidden := forbiddenMongoStages[strings.ToLower(key)]; forbidden {
				return fmt.Errorf("pipeline stage %q is not allowed in read-only queries", key)
			}
		}
	}
	return nil
}

func (r *mongoRunner) runFind(ctx context.Context, collection *mongo.Collection, mq *mongoQuery, limit int) (QueryOutput, error) {
	opts := options.Find()
	if mq.Projection != nil {
		opts.SetProjection(mq.Projection)
	}
	if mq.Sort != nil {
		opts.SetSort(mq.Sort)
	}
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}

	cursor, err := collection.Find(ctx, mq.Filter, opts)
	if err != nil {
		return QueryOutput{}, fmt.Errorf("find: %w", err)
	}
	defer cursor.Close(ctx)

	return collectMongoCursor(ctx, cursor, limit)
}

func (r *mongoRunner) runAggregate(ctx context.Context, collection *mongo.Collection, mq *mongoQuery, limit int) (QueryOutput, error) {
	pipeline := make([]interface{}, len(mq.Pipeline))
	for i, stage := range mq.Pipeline {
		pipeline[i] = stage
	}
	if limit > 0 {
		pipeline = append(pipeline, bson.M{"$limit": limit})
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return QueryOutput{}, fmt.Errorf("aggregate: %w", err)
	}
	defer cursor.Close(ctx)

	return collectMongoCursor(ctx, cursor, limit)
}

func collectMongoCursor(ctx context.Context, cursor *mongo.Cursor, limit int) (QueryOutput, error) {
	output := QueryOutput{
		Rows: make([]map[string]interface{}, 0),
	}
	columnSet := make(map[string]struct{})

	for cursor.Next(ctx) {
		if limit > 0 && output.RowCount >= limit {
			output.Truncated = true
			break
		}

		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			return QueryOutput{}, fmt.Errorf("decode document: %w", err)
		}

		row := make(map[string]interface{}, len(doc))
		for key, value := range doc {
			columnSet[key] = struct{}{}
			row[key] = normalizeMongoValue(value)
		}

		output.Rows = append(output.Rows, row)
		output.RowCount++
	}
	if err := cursor.Err(); err != nil {
		return QueryOutput{}, fmt.Errorf("cursor: %w", err)
	}

	output.Columns = make([]string, 0, len(columnSet))
	for col := range columnSet {
		output.Columns = append(output.Columns, col)
	}
	sort.Strings(output.Columns)

	return output, nil
}

func normalizeMongoValue(value interface{}) interface{} {
	switch v := value.(type) {
	case nil:
		return nil
	case primitive.ObjectID:
		return v.Hex()
	case primitive.DateTime:
		return v.Time().UTC().Format(time.RFC3339Nano)
	case primitive.Decimal128:
		return v.String()
	case primitive.Binary:
		return fmt.Sprintf("%x", v.Data)
	case primitive.Regex:
		return v.Pattern
	case bson.M:
		out := make(map[string]interface{}, len(v))
		for key, item := range v {
			out[key] = normalizeMongoValue(item)
		}
		return out
	case bson.A:
		out := make([]interface{}, len(v))
		for i, item := range v {
			out[i] = normalizeMongoValue(item)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, item := range v {
			out[i] = normalizeMongoValue(item)
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for key, item := range v {
			out[key] = normalizeMongoValue(item)
		}
		return out
	default:
		if normalized := normalizeSQLValue(v); normalized != v {
			return normalized
		}
		return v
	}
}
