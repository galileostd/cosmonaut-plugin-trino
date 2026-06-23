// Package plugin implements the Cosmonaut plugin for Apache Trino.
package plugin

import (
	"context"
	"fmt"
	"log/slog"

	pluginv1 "github.com/galileostd/cosmonaut-sdk/go/plugin/v1"
	sdkserver "github.com/galileostd/cosmonaut-sdk/go/server"
	"github.com/galileostd/cosmonaut-plugin-trino/internal/client"
)

// Plugin implements the Cosmonaut PluginService for Apache Trino.
// It embeds UnimplementedPlugin to get default responses for unimplemented RPCs.
type Plugin struct {
	sdkserver.UnimplementedPlugin
}

// New creates a new Trino plugin instance.
func New() *Plugin {
	return &Plugin{}
}

// Describe returns static metadata about this plugin.
func (p *Plugin) Describe(_ context.Context, _ *pluginv1.DescribeRequest) (*pluginv1.DescribeResponse, error) {
	return &pluginv1.DescribeResponse{
		PluginName:    "trino",
		DisplayName:   "Apache Trino",
		Version:       "v0.1.0",
		Description:   "Distributed SQL query engine for the lakehouse. Connects to Iceberg tables via Polaris catalog.",
		PluginType:    pluginv1.PluginType_PLUGIN_TYPE_QUERY_ENGINE,
		ExecutionType: pluginv1.ExecutionType_EXECUTION_TYPE_QUERY,
		Capabilities: []*pluginv1.Capability{
			{
				Type:        "query",
				Description: "Execute SQL queries against Iceberg tables via Trino.",
			},
			{
				Type:        "list-jobs",
				Description: "List running and recent queries.",
			},
			{
				Type:        "kill-job",
				Description: "Cancel a running query.",
			},
		},
	}, nil
}

// HealthCheck verifies that Trino is reachable and not starting up.
func (p *Plugin) HealthCheck(ctx context.Context, req *pluginv1.HealthCheckRequest) (*pluginv1.HealthCheckResponse, error) {
	if req.Component == nil {
		return unhealthy("component is required"), nil
	}

	trinoClient := clientFromComponent(req.Component)

	info, err := trinoClient.Info(ctx)
	if err != nil {
		slog.Warn("trino health check failed",
			"component", req.Component.Name,
			"endpoint", req.Component.Endpoint,
			"err", err,
		)
		return &pluginv1.HealthCheckResponse{
			State:   pluginv1.HealthState_HEALTH_STATE_UNHEALTHY,
			Message: fmt.Sprintf("failed to reach Trino at %s: %v", req.Component.Endpoint, err),
		}, nil
	}

	if info.Starting {
		return &pluginv1.HealthCheckResponse{
			State:   pluginv1.HealthState_HEALTH_STATE_DEGRADED,
			Message: "Trino is still starting up",
			Details: map[string]string{
				"version":     info.NodeVersion.Version,
				"environment": info.Environment,
			},
		}, nil
	}

	return &pluginv1.HealthCheckResponse{
		State:   pluginv1.HealthState_HEALTH_STATE_HEALTHY,
		Message: fmt.Sprintf("Trino is healthy (version %s)", info.NodeVersion.Version),
		Details: map[string]string{
			"version":     info.NodeVersion.Version,
			"environment": info.Environment,
			"uptime":      info.Uptime,
		},
	}, nil
}

// Execute runs an action on Trino.
// Supported actions: "query", "list-jobs", "kill-job"
func (p *Plugin) Execute(ctx context.Context, req *pluginv1.ExecuteRequest) (*pluginv1.ExecuteResponse, error) {
	if req.Component == nil {
		return nil, fmt.Errorf("component is required")
	}

	switch req.Action {
	case "query":
		return p.executeQuery(ctx, req)
	case "list-jobs":
		return p.listJobs(ctx, req)
	case "kill-job":
		return p.killJob(ctx, req)
	default:
		return &pluginv1.ExecuteResponse{
			State:   pluginv1.JobState_JOB_STATE_FAILED,
			Message: fmt.Sprintf("unsupported action: %s", req.Action),
		}, nil
	}
}

// GetJob returns the current status of a Trino query.
func (p *Plugin) GetJob(ctx context.Context, req *pluginv1.GetJobRequest) (*pluginv1.GetJobResponse, error) {
	if req.Component == nil {
		return nil, fmt.Errorf("component is required")
	}

	trinoClient := clientFromComponent(req.Component)

	status, err := trinoClient.QueryStatus(ctx, req.JobId)
	if err != nil {
		return &pluginv1.GetJobResponse{
			JobId:   req.JobId,
			State:   pluginv1.JobState_JOB_STATE_UNKNOWN,
			Message: err.Error(),
		}, nil
	}

	return &pluginv1.GetJobResponse{
		JobId:   status.QueryID,
		State:   trinoStateToJobState(status.State),
		Message: status.State,
		Details: map[string]string{
			"state":           status.State,
			"elapsed_ms":      fmt.Sprintf("%.0f", status.Stats.ElapsedTimeMs),
			"total_rows":      fmt.Sprintf("%d", status.Stats.TotalRows),
			"processed_rows":  fmt.Sprintf("%d", status.Stats.ProcessedRows),
			"processed_bytes": fmt.Sprintf("%d", status.Stats.ProcessedBytes),
		},
	}, nil
}

// CancelJob cancels a running Trino query.
func (p *Plugin) CancelJob(ctx context.Context, req *pluginv1.CancelJobRequest) (*pluginv1.CancelJobResponse, error) {
	if req.Component == nil {
		return nil, fmt.Errorf("component is required")
	}

	trinoClient := clientFromComponent(req.Component)

	if err := trinoClient.CancelQuery(ctx, req.JobId); err != nil {
		return &pluginv1.CancelJobResponse{
			Canceled: false,
			Message:  err.Error(),
		}, nil
	}

	return &pluginv1.CancelJobResponse{
		Canceled: true,
		Message:  fmt.Sprintf("query %s canceled", req.JobId),
	}, nil
}

// ── private helpers ────────────────────────────────────────────────────────────

func (p *Plugin) executeQuery(ctx context.Context, req *pluginv1.ExecuteRequest) (*pluginv1.ExecuteResponse, error) {
	sql, ok := req.Payload["sql"]
	if !ok || sql == "" {
		return &pluginv1.ExecuteResponse{
			State:   pluginv1.JobState_JOB_STATE_FAILED,
			Message: "missing required payload field: sql",
		}, nil
	}

	trinoClient := clientFromComponent(req.Component)

	qr, err := trinoClient.SubmitQuery(ctx, sql)
	if err != nil {
		return &pluginv1.ExecuteResponse{
			State:   pluginv1.JobState_JOB_STATE_FAILED,
			Message: fmt.Sprintf("failed to submit query: %v", err),
		}, nil
	}

	slog.Info("trino query submitted",
		"query_id", qr.ID,
		"component", req.Component.Name,
	)

	return &pluginv1.ExecuteResponse{
		JobId:   qr.ID,
		State:   pluginv1.JobState_JOB_STATE_RUNNING,
		Message: fmt.Sprintf("query submitted with ID %s", qr.ID),
		Result: map[string]string{
			"query_id": qr.ID,
			"info_uri": qr.InfoURI,
		},
	}, nil
}

func (p *Plugin) listJobs(ctx context.Context, req *pluginv1.ExecuteRequest) (*pluginv1.ExecuteResponse, error) {
	// Trino doesn't have a simple "list all queries" endpoint in the same way
	// that Spark does with SparkApplications. The /v1/query endpoint lists
	// queries but requires Trino admin privileges.
	// For now we return a placeholder — this will be expanded when we add
	// catalog-level query history via OpenMetadata.
	return &pluginv1.ExecuteResponse{
		JobId:   "",
		State:   pluginv1.JobState_JOB_STATE_SUCCEEDED,
		Message: "list-jobs not yet implemented for Trino — use the Trino UI for query history",
	}, nil
}

func (p *Plugin) killJob(ctx context.Context, req *pluginv1.ExecuteRequest) (*pluginv1.ExecuteResponse, error) {
	queryID, ok := req.Payload["job_id"]
	if !ok || queryID == "" {
		return &pluginv1.ExecuteResponse{
			State:   pluginv1.JobState_JOB_STATE_FAILED,
			Message: "missing required payload field: job_id",
		}, nil
	}

	trinoClient := clientFromComponent(req.Component)

	if err := trinoClient.CancelQuery(ctx, queryID); err != nil {
		return &pluginv1.ExecuteResponse{
			State:   pluginv1.JobState_JOB_STATE_FAILED,
			Message: fmt.Sprintf("failed to cancel query %s: %v", queryID, err),
		}, nil
	}

	return &pluginv1.ExecuteResponse{
		JobId:   queryID,
		State:   pluginv1.JobState_JOB_STATE_CANCELED,
		Message: fmt.Sprintf("query %s canceled", queryID),
	}, nil
}

// clientFromComponent builds a Trino client from a Component.
// The user can be overridden via component.Config["user"].
func clientFromComponent(component *pluginv1.Component) *client.Client {
	user := component.Config["user"]
	return client.New(component.Endpoint, user)
}

// trinoStateToJobState maps Trino query states to Cosmonaut job states.
func trinoStateToJobState(state string) pluginv1.JobState {
	switch state {
	case "QUEUED", "WAITING_FOR_PREREQUISITES", "DISPATCHING", "PLANNING":
		return pluginv1.JobState_JOB_STATE_PENDING
	case "STARTING", "RUNNING":
		return pluginv1.JobState_JOB_STATE_RUNNING
	case "FINISHING", "FINISHED":
		return pluginv1.JobState_JOB_STATE_SUCCEEDED
	case "FAILED":
		return pluginv1.JobState_JOB_STATE_FAILED
	case "CANCELED":
		return pluginv1.JobState_JOB_STATE_CANCELED
	default:
		return pluginv1.JobState_JOB_STATE_UNKNOWN
	}
}

func unhealthy(msg string) *pluginv1.HealthCheckResponse {
	return &pluginv1.HealthCheckResponse{
		State:   pluginv1.HealthState_HEALTH_STATE_UNHEALTHY,
		Message: msg,
	}
}
