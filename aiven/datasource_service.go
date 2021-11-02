// Copyright (c) 2017 jelmersnoeck
// Copyright (c) 2018-2021 Aiven, Helsinki, Finland. https://aiven.io/
package aiven

import (
	"context"

	"github.com/aiven/aiven-go-client"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func datasourceService() *schema.Resource {
	return &schema.Resource{
		ReadContext:        datasourceServiceRead,
		Description:        "The Service datasource provides information about specific Aiven Services.",
		DeprecationMessage: "Please use the specific service datasources instead of this datasource.",
		Schema:             resourceSchemaAsDatasourceSchema(aivenServiceSchema, "project", "service_name"),
	}
}

func datasourceServiceRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	projectName := d.Get("project").(string)
	serviceName := d.Get("service_name").(string)
	d.SetId(buildResourceID(projectName, serviceName))

	services, err := client.Services.List(projectName)
	if err != nil {
		return diag.FromErr(err)
	}

	for _, service := range services {
		if service.Name == serviceName {
			// at this point we need to set the required parameters, so that we can validate
			// fetch the service plan parameters from the API and validate the disk space
			if err := d.Set("service_type", service.Type); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set("cloud_name", service.CloudName); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set("plan", service.Plan); err != nil {
				return diag.FromErr(err)
			}
			return resourceServiceRead(ctx, d, m)
		}
	}

	return diag.Errorf("service %s/%s not found", projectName, serviceName)
}
