package terraform

import (
	"fmt"
	"strings"

	bifrost "github.com/arkeros/senku/devtools/bifrost/api"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

func Render(spec bifrost.Workload) ([]byte, error) {
	switch spec.Kind {
	case bifrost.KindService:
		return renderService(spec)
	case bifrost.KindCronJob:
		return renderCronJob(spec)
	default:
		return nil, fmt.Errorf("unsupported kind %q", spec.Kind)
	}
}

func renderService(spec bifrost.Workload) ([]byte, error) {
	projectID := spec.Spec.GCP.ProjectID
	accountID, err := accountIDFromEmail(spec.Spec.ServiceAccountName)
	if err != nil {
		return nil, err
	}
	serviceAccountResourceName := terraformIdentifier(accountID)

	file := hclwrite.NewEmptyFile()
	body := file.Body()

	serviceAccountBlock := body.AppendNewBlock("resource", []string{"google_service_account", serviceAccountResourceName})
	serviceAccountBody := serviceAccountBlock.Body()
	serviceAccountBody.SetAttributeValue("project", cty.StringVal(projectID))
	serviceAccountBody.SetAttributeValue("account_id", cty.StringVal(accountID))
	serviceAccountBody.SetAttributeValue("display_name", cty.StringVal("Runtime identity for "+spec.Metadata.Name))

	if spec.Spec.Kubernetes != nil {
		kubernetesServiceAccountName := spec.Metadata.Name
		namespace := spec.Spec.Kubernetes.Namespace
		workloadIdentityResourceName := terraformIdentifier(accountID + "_workload_identity")
		serviceAccountTraversal, err := traversalForExpr("google_service_account." + serviceAccountResourceName + ".name")
		if err != nil {
			return nil, err
		}

		body.AppendNewline()

		workloadIdentityBlock := body.AppendNewBlock("resource", []string{"google_service_account_iam_member", workloadIdentityResourceName})
		workloadIdentityBody := workloadIdentityBlock.Body()
		workloadIdentityBody.SetAttributeTraversal("service_account_id", serviceAccountTraversal)
		workloadIdentityBody.SetAttributeValue("role", cty.StringVal("roles/iam.workloadIdentityUser"))
		workloadIdentityBody.SetAttributeValue(
			"member",
			cty.StringVal(fmt.Sprintf("serviceAccount:%s.svc.id.goog[%s/%s]", projectID, namespace, kubernetesServiceAccountName)),
		)
	}

	return hclwrite.Format(file.Bytes()), nil
}

func renderCronJob(spec bifrost.Workload) ([]byte, error) {
	projectID := spec.Spec.GCP.ProjectID
	runtimeAccountID, err := accountIDFromEmail(spec.Spec.ServiceAccountName)
	if err != nil {
		return nil, err
	}
	runtimeResourceName := terraformIdentifier(runtimeAccountID)
	schedulerAccountID, err := bifrost.DefaultServiceAccountAccountID("sch-", spec.Metadata.Name)
	if err != nil {
		return nil, err
	}
	schedulerResourceName := terraformIdentifier(schedulerAccountID)
	schedulerEmailTraversal, err := traversalForExpr("google_service_account." + schedulerResourceName + ".email")
	if err != nil {
		return nil, err
	}

	file := hclwrite.NewEmptyFile()
	body := file.Body()

	runtimeServiceAccountBlock := body.AppendNewBlock("resource", []string{"google_service_account", runtimeResourceName})
	runtimeServiceAccountBody := runtimeServiceAccountBlock.Body()
	runtimeServiceAccountBody.SetAttributeValue("project", cty.StringVal(projectID))
	runtimeServiceAccountBody.SetAttributeValue("account_id", cty.StringVal(runtimeAccountID))
	runtimeServiceAccountBody.SetAttributeValue("display_name", cty.StringVal("Runtime identity for "+spec.Metadata.Name))

	if spec.Spec.Kubernetes != nil {
		kubernetesServiceAccountName := spec.Metadata.Name
		namespace := spec.Spec.Kubernetes.Namespace
		workloadIdentityResourceName := terraformIdentifier(runtimeAccountID + "_workload_identity")
		runtimeServiceAccountTraversal, err := traversalForExpr("google_service_account." + runtimeResourceName + ".name")
		if err != nil {
			return nil, err
		}

		body.AppendNewline()

		workloadIdentityBlock := body.AppendNewBlock("resource", []string{"google_service_account_iam_member", workloadIdentityResourceName})
		workloadIdentityBody := workloadIdentityBlock.Body()
		workloadIdentityBody.SetAttributeTraversal("service_account_id", runtimeServiceAccountTraversal)
		workloadIdentityBody.SetAttributeValue("role", cty.StringVal("roles/iam.workloadIdentityUser"))
		workloadIdentityBody.SetAttributeValue(
			"member",
			cty.StringVal(fmt.Sprintf("serviceAccount:%s.svc.id.goog[%s/%s]", projectID, namespace, kubernetesServiceAccountName)),
		)
	}

	body.AppendNewline()

	schedulerServiceAccountBlock := body.AppendNewBlock("resource", []string{"google_service_account", schedulerResourceName})
	schedulerServiceAccountBody := schedulerServiceAccountBlock.Body()
	schedulerServiceAccountBody.SetAttributeValue("project", cty.StringVal(projectID))
	schedulerServiceAccountBody.SetAttributeValue("account_id", cty.StringVal(schedulerAccountID))
	schedulerServiceAccountBody.SetAttributeValue("display_name", cty.StringVal("Cloud Scheduler invoker for "+spec.Metadata.Name))

	body.AppendNewline()

	schedulerInvokerBlock := body.AppendNewBlock("resource", []string{"google_project_iam_member", terraformIdentifier(schedulerAccountID + "_run_invoker")})
	schedulerInvokerBody := schedulerInvokerBlock.Body()
	schedulerInvokerBody.SetAttributeValue("project", cty.StringVal(projectID))
	schedulerInvokerBody.SetAttributeValue("role", cty.StringVal("roles/run.invoker"))
	schedulerInvokerBody.SetAttributeRaw(
		"member",
		hclwrite.TokensForFunctionCall(
			"format",
			hclwrite.TokensForValue(cty.StringVal("serviceAccount:%s")),
			hclwrite.TokensForTraversal(schedulerEmailTraversal),
		),
	)

	body.AppendNewline()

	schedulerBlock := body.AppendNewBlock("resource", []string{"google_cloud_scheduler_job", terraformIdentifier(spec.Metadata.Name + "_schedule")})
	schedulerBody := schedulerBlock.Body()
	schedulerBody.SetAttributeValue("project", cty.StringVal(projectID))
	schedulerBody.SetAttributeValue("name", cty.StringVal(spec.Metadata.Name))
	schedulerBody.SetAttributeValue("region", cty.StringVal(spec.Spec.GCP.Region))
	schedulerBody.SetAttributeValue("schedule", cty.StringVal(spec.Spec.Schedule.Cron))
	if spec.Spec.Schedule.TimeZone != "" {
		schedulerBody.SetAttributeValue("time_zone", cty.StringVal(spec.Spec.Schedule.TimeZone))
	}
	if spec.Spec.GCP.CloudScheduler.AttemptDeadlineSeconds > 0 {
		schedulerBody.SetAttributeValue("attempt_deadline", cty.StringVal(fmt.Sprintf("%ds", spec.Spec.GCP.CloudScheduler.AttemptDeadlineSeconds)))
	}
	if spec.Spec.GCP.CloudScheduler.RetryCount > 0 {
		retryConfig := schedulerBody.AppendNewBlock("retry_config", nil)
		retryConfig.Body().SetAttributeValue("retry_count", cty.NumberIntVal(int64(spec.Spec.GCP.CloudScheduler.RetryCount)))
	}
	httpTarget := schedulerBody.AppendNewBlock("http_target", nil)
	httpTargetBody := httpTarget.Body()
	httpTargetBody.SetAttributeValue("http_method", cty.StringVal("POST"))
	httpTargetBody.SetAttributeValue("uri", cty.StringVal(fmt.Sprintf("https://run.googleapis.com/v2/projects/%s/locations/%s/jobs/%s:run", projectID, spec.Spec.GCP.Region, spec.Metadata.Name)))
	httpTargetBody.SetAttributeRaw("body", hclwrite.TokensForFunctionCall("base64encode", hclwrite.TokensForValue(cty.StringVal("{}"))))
	httpTargetBody.SetAttributeValue("headers", cty.MapVal(map[string]cty.Value{
		"Content-Type": cty.StringVal("application/json"),
	}))
	oauthToken := httpTargetBody.AppendNewBlock("oauth_token", nil)
	oauthToken.Body().SetAttributeTraversal("service_account_email", schedulerEmailTraversal)

	return hclwrite.Format(file.Bytes()), nil
}

func accountIDFromEmail(email string) (string, error) {
	local, _, ok := strings.Cut(strings.TrimSpace(email), "@")
	if !ok || local == "" {
		return "", fmt.Errorf("invalid service account email %q", email)
	}
	if len(local) < 6 || len(local) > 30 {
		return "", fmt.Errorf("account ID %q must be between 6 and 30 characters", local)
	}
	return local, nil
}

func terraformIdentifier(name string) string {
	var out strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			out.WriteRune(r + ('a' - 'A'))
		case r == '-', r == '.', r == '/', r == '_':
			out.WriteByte('_')
		}
	}
	if out.Len() == 0 {
		return "service"
	}
	result := out.String()
	if result[0] >= '0' && result[0] <= '9' {
		return "service_" + result
	}
	return result
}

func traversalForExpr(expr string) (hcl.Traversal, error) {
	parts := strings.Split(expr, ".")
	if len(parts) == 0 || parts[0] == "" {
		return nil, fmt.Errorf("expression must be a dot-separated traversal")
	}

	traversal := hcl.Traversal{
		hcl.TraverseRoot{Name: parts[0]},
	}
	for _, part := range parts[1:] {
		if part == "" {
			return nil, fmt.Errorf("expression must be a dot-separated traversal")
		}
		traversal = append(traversal, hcl.TraverseAttr{Name: part})
	}
	return traversal, nil
}
