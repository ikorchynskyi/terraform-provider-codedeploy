// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/codedeploy"
	codedeployTypes "github.com/aws/aws-sdk-go-v2/service/codedeploy/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceDeployment() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceDeploymentCreate,
		ReadContext:   resourceDeploymentRead,
		UpdateContext: resourceDeploymentUpdate,
		DeleteContext: resourceDeploymentDelete,
		Schema: map[string]*schema.Schema{
			"application_name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of the CodeDeploy application",
			},
			"deployment_group_name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of the CodeDeploy deployment group",
			},
			"revision": {
				Type:        schema.TypeList,
				Required:    true,
				ForceNew:    true,
				MaxItems:    1,
				Description: "Revision details for the deployment",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"revision_type": {
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
							Description: "Type of the revision",
							ValidateFunc: validation.StringInSlice([]string{
								string(codedeployTypes.RevisionLocationTypeS3),
								string(codedeployTypes.RevisionLocationTypeGitHub),
								string(codedeployTypes.RevisionLocationTypeAppSpecContent),
							}, false),
						},
						"s3_location": {
							Type:        schema.TypeList,
							Optional:    true,
							ForceNew:    true,
							MaxItems:    1,
							Description: "S3 location details for the revision",
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"bucket": {
										Type:        schema.TypeString,
										Required:    true,
										Description: "Name of the S3 bucket",
									},
									"key": {
										Type:        schema.TypeString,
										Required:    true,
										Description: "Key of the S3 object",
									},
									"bundle_type": {
										Type:        schema.TypeString,
										Required:    true,
										Description: "Type of the revision bundle",
										ValidateFunc: validation.StringInSlice([]string{
											"zip",
											"tar",
										}, false),
									},
								},
							},
						},
						"github_location": {
							Type:        schema.TypeList,
							Optional:    true,
							ForceNew:    true,
							MaxItems:    1,
							Description: "GitHub location details for the revision",
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"repository": {
										Type:        schema.TypeString,
										Required:    true,
										Description: "Name of the GitHub repository",
									},
									"commit_id": {
										Type:        schema.TypeString,
										Required:    true,
										Description: "Commit ID of the revision",
									},
								},
							},
						},
						"appspec_content": {
							Type:        schema.TypeList,
							Optional:    true,
							ForceNew:    true,
							MaxItems:    1,
							Description: "AppSpec location details for the revision",
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"content": {
										Type:        schema.TypeString,
										Required:    true,
										Description: "The YAML-formatted or JSON-formatted revision string",
									},
									"sha256": {
										Type:        schema.TypeString,
										Optional:    true,
										Description: "The SHA256 hash value of the revision content",
									},
								},
							},
						},
					},
				},
			},
			"auto_rollback_enabled": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "Indicates whether a failed deployment should be rolled back",
			},
			"deployment_status": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Status of the deployment",
			},
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(60 * time.Minute),
		},
	}
}

func resourceDeploymentCreate(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	svc := m.(*codedeploy.Client)

	// Read and validate the input parameters
	applicationName := d.Get("application_name").(string)
	deploymentGroupName := d.Get("deployment_group_name").(string)
	revision := d.Get("revision").([]any)[0].(map[string]any)
	revisionType := revision["revision_type"].(string)

	// Prepare the deployment input parameters based on the revision type
	var input codedeploy.CreateDeploymentInput
	switch revisionType {
	case string(codedeployTypes.RevisionLocationTypeS3):
		s3Location := revision["s3_location"].([]any)[0].(map[string]any)
		input = prepareDeploymentInputWithS3Location(applicationName, deploymentGroupName, s3Location)
	case string(codedeployTypes.RevisionLocationTypeGitHub):
		githubLocation := revision["github_location"].([]any)[0].(map[string]any)
		input = prepareDeploymentInputWithGitHubLocation(applicationName, deploymentGroupName, githubLocation)
	case string(codedeployTypes.RevisionLocationTypeAppSpecContent):
		appSpecContent := revision["appspec_content"].([]any)[0].(map[string]any)
		input = prepareDeploymentInputWithAppSpecContent(applicationName, deploymentGroupName, appSpecContent)
	}

	// Create the deployment
	deployment, err := svc.CreateDeployment(ctx, &input)
	if err != nil {
		return diag.FromErr(err)
	}

	// Wait for the deployment to complete or timeout
	delay := 15 * time.Second
	waiter := codedeploy.NewDeploymentSuccessfulWaiter(svc, func(o *codedeploy.DeploymentSuccessfulWaiterOptions) {
		o.MaxDelay = delay
	})
	output, err := waiter.WaitForOutput(ctx, &codedeploy.GetDeploymentInput{
		DeploymentId: deployment.DeploymentId,
	}, d.Timeout(schema.TimeoutCreate)-delay)
	if err != nil {
		diags := diag.FromErr(err)
		if output == nil {
			_, err = svc.StopDeployment(ctx, &codedeploy.StopDeploymentInput{
				DeploymentId:        deployment.DeploymentId,
				AutoRollbackEnabled: aws.Bool(d.Get("auto_rollback_enabled").(bool)),
			})
			if err != nil {
				diags = append(diags, diag.FromErr(err)...)
			}
		}
		return diags
	}

	// Store the deployment ID in the resource data
	d.SetId(*deployment.DeploymentId)

	return resourceDeploymentRead(ctx, d, m)
}

func resourceDeploymentRead(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	svc := m.(*codedeploy.Client)

	// Read the deployment ID from the resource data
	deploymentID := d.Id()

	// Fetch the deployment
	deployment, err := svc.GetDeployment(ctx, &codedeploy.GetDeploymentInput{
		DeploymentId: aws.String(deploymentID),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	// Update the resource data with the deployment status
	if d.Set("deployment_status", deployment.DeploymentInfo.Status) != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceDeploymentUpdate(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	// Nothing to do for update
	return nil
}

func resourceDeploymentDelete(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	svc := m.(*codedeploy.Client)

	// Read the deployment ID from the resource data
	deploymentID := d.Id()

	// Fetch the deployment
	deployment, err := svc.GetDeployment(ctx, &codedeploy.GetDeploymentInput{
		DeploymentId: aws.String(deploymentID),
	})
	if err != nil {
		return diag.Diagnostics{
			diag.Diagnostic{
				Severity: diag.Warning,
				Summary:  err.Error(),
			},
		}
	}

	status := deployment.DeploymentInfo.Status
	if status != codedeployTypes.DeploymentStatusSucceeded &&
		status != codedeployTypes.DeploymentStatusFailed &&
		status != codedeployTypes.DeploymentStatusStopped {
		_, err = svc.StopDeployment(ctx, &codedeploy.StopDeploymentInput{
			DeploymentId:        deployment.DeploymentInfo.DeploymentId,
			AutoRollbackEnabled: aws.Bool(d.Get("auto_rollback_enabled").(bool)),
		})
		if err != nil {
			return diag.Diagnostics{
				diag.Diagnostic{
					Severity: diag.Warning,
					Summary:  err.Error(),
				},
			}
		}
	}

	return nil
}

func prepareDeploymentInputWithS3Location(applicationName, deploymentGroupName string, s3Location map[string]any) codedeploy.CreateDeploymentInput {
	return codedeploy.CreateDeploymentInput{
		ApplicationName:     aws.String(applicationName),
		DeploymentGroupName: aws.String(deploymentGroupName),
		Revision: &codedeployTypes.RevisionLocation{
			RevisionType: codedeployTypes.RevisionLocationTypeS3,
			S3Location: &codedeployTypes.S3Location{
				Bucket:     aws.String(s3Location["bucket"].(string)),
				Key:        aws.String(s3Location["key"].(string)),
				BundleType: s3Location["bundle_type"].(codedeployTypes.BundleType),
			},
		},
	}
}

func prepareDeploymentInputWithGitHubLocation(applicationName, deploymentGroupName string, githubLocation map[string]any) codedeploy.CreateDeploymentInput {
	return codedeploy.CreateDeploymentInput{
		ApplicationName:     aws.String(applicationName),
		DeploymentGroupName: aws.String(deploymentGroupName),
		Revision: &codedeployTypes.RevisionLocation{
			RevisionType: codedeployTypes.RevisionLocationTypeGitHub,
			GitHubLocation: &codedeployTypes.GitHubLocation{
				Repository: aws.String(githubLocation["repository"].(string)),
				CommitId:   aws.String(githubLocation["commit_id"].(string)),
			},
		},
	}
}

func prepareDeploymentInputWithAppSpecContent(applicationName, deploymentGroupName string, appSpecContent map[string]any) codedeploy.CreateDeploymentInput {
	content := appSpecContent["content"].(string)
	var sha256Hash string
	if appSpecContent["sha256"] == nil {
		hash := sha256.Sum256([]byte(content))
		sha256Hash = hex.EncodeToString(hash[:])
	} else {
		sha256Hash = appSpecContent["sha256"].(string)
	}

	return codedeploy.CreateDeploymentInput{
		ApplicationName:     aws.String(applicationName),
		DeploymentGroupName: aws.String(deploymentGroupName),
		Revision: &codedeployTypes.RevisionLocation{
			RevisionType: codedeployTypes.RevisionLocationTypeAppSpecContent,
			AppSpecContent: &codedeployTypes.AppSpecContent{
				Content: aws.String(content),
				Sha256:  aws.String(sha256Hash),
			},
		},
	}
}
