package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/ETretyakov/rag-with-tables/internal/query"
)

// QueryDeps carries dependencies for the POST /query endpoint.
type QueryDeps struct {
	Pipeline *query.Pipeline
}

type QueryInput struct {
	Body struct {
		Query string `json:"query"  minLength:"1" doc:"Natural-language question about the indexed data"`
		TopK  int    `json:"top_k"  minimum:"1"  maximum:"50" default:"10" doc:"Number of chunks to retrieve from Qdrant"`
	}
}

type QueryOutput struct {
	Body query.QueryResult
}

func registerQueryRoutes(api huma.API, deps QueryDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "query",
		Method:      http.MethodPost,
		Path:        "/query",
		Summary:     "Ask a natural-language question about the indexed tables",
		Tags:        []string{"Query"},
	}, func(ctx context.Context, input *QueryInput) (*QueryOutput, error) {
		if deps.Pipeline == nil {
			return nil, huma.Error503ServiceUnavailable("query pipeline not configured")
		}

		topK := input.Body.TopK
		if topK == 0 {
			topK = 10
		}

		result, err := deps.Pipeline.Query(ctx, query.QueryRequest{
			Query: input.Body.Query,
			TopK:  topK,
		})
		if err != nil {
			return nil, huma.Error500InternalServerError("query failed: " + err.Error())
		}

		return &QueryOutput{Body: *result}, nil
	})
}
