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

func resourceDeploymentCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	svc := m.(*codedeploy.Client)

	// Read and validate the input parameters
	applicationName := d.Get("application_name").(string)
	deploymentGroupName := d.Get("deployment_group_name").(string)
	revisionType := d.Get("revision").([]interface{})[0].(map[string]interface{})["revision_type"].(string)

	// Prepare the deployment input parameters based on the revision type
	var input codedeploy.CreateDeploymentInput
	switch revisionType {
	case string(codedeployTypes.RevisionLocationTypeS3):
		s3Location := d.Get("revision").([]interface{})[0].(map[string]interface{})["s3_location"].([]interface{})[0].(map[string]interface{})
		input = prepareDeploymentInputWithS3Location(applicationName, deploymentGroupName, s3Location)
	case string(codedeployTypes.RevisionLocationTypeGitHub):
		githubLocation := d.Get("revision").([]interface{})[0].(map[string]interface{})["github_location"].([]interface{})[0].(map[string]interface{})
		input = prepareDeploymentInputWithGitHubLocation(applicationName, deploymentGroupName, githubLocation)
	case string(codedeployTypes.RevisionLocationTypeAppSpecContent):
		appSpecContent := d.Get("revision").([]interface{})[0].(map[string]interface{})["appspec_content"].([]interface{})[0].(map[string]interface{})
		input = prepareDeploymentInputWithAppSpecContent(applicationName, deploymentGroupName, appSpecContent)
	}

	// Create the deployment
	result, err := svc.CreateDeployment(ctx, &input)
	if err != nil {
		return diag.FromErr(err)
	}

	// Wait for the deployment to complete or timeout
	waiter := codedeploy.NewDeploymentSuccessfulWaiter(svc, func(o *codedeploy.DeploymentSuccessfulWaiterOptions) {
		o.MaxDelay = 30 * time.Second
	})
	_, err = waiter.WaitForOutput(ctx, &codedeploy.GetDeploymentInput{
		DeploymentId: result.DeploymentId,
	}, d.Timeout(schema.TimeoutCreate))
	if err != nil {
		return diag.FromErr(err)
	}

	// Store the deployment ID in the resource data
	d.SetId(*result.DeploymentId)

	return resourceDeploymentRead(ctx, d, m)
}

func resourceDeploymentRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	svc := m.(*codedeploy.Client)

	// Read the deployment ID from the resource data
	deploymentID := d.Id()

	// Describe the deployment
	result, err := svc.GetDeployment(ctx, &codedeploy.GetDeploymentInput{
		DeploymentId: aws.String(deploymentID),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	// Update the resource data with the deployment status
	d.Set("deployment_status", result.DeploymentInfo.Status)

	return nil
}

func resourceDeploymentDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	// Nothing to do for deletion
	return nil
}

func prepareDeploymentInputWithS3Location(applicationName, deploymentGroupName string, s3Location map[string]interface{}) codedeploy.CreateDeploymentInput {
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

func prepareDeploymentInputWithGitHubLocation(applicationName, deploymentGroupName string, githubLocation map[string]interface{}) codedeploy.CreateDeploymentInput {
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

func prepareDeploymentInputWithAppSpecContent(applicationName, deploymentGroupName string, appSpecContent map[string]interface{}) codedeploy.CreateDeploymentInput {
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
