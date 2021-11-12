// Copyright (c) 2017 jelmersnoeck
// Copyright (c) 2018-2021 Aiven, Helsinki, Finland. https://aiven.io/
package aiven

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/docker/go-units"

	"github.com/aiven/aiven-go-client"
	"github.com/aiven/terraform-provider-aiven/pkg/ipfilter"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

const (
	ServiceTypePG               = "pg"
	ServiceTypeCassandra        = "cassandra"
	ServiceTypeElasticsearch    = "elasticsearch"
	ServiceTypeOpensearch       = "opensearch"
	ServiceTypeGrafana          = "grafana"
	ServiceTypeInfluxDB         = "influxdb"
	ServiceTypeRedis            = "redis"
	ServiceTypeMySQL            = "mysql"
	ServiceTypeKafka            = "kafka"
	ServiceTypeKafkaConnect     = "kafka_connect"
	ServiceTypeKafkaMirrormaker = "kafka_mirrormaker"
	ServiceTypeM3               = "m3db"
	ServiceTypeM3Aggregator     = "m3aggregator"
	ServiceTypeFlink            = "flink"
)

func availableServiceTypes() []string {
	return []string{
		ServiceTypePG,
		ServiceTypeCassandra,
		ServiceTypeElasticsearch,
		ServiceTypeGrafana,
		ServiceTypeInfluxDB,
		ServiceTypeRedis,
		ServiceTypeMySQL,
		ServiceTypeKafka,
		ServiceTypeKafkaConnect,
		ServiceTypeKafkaMirrormaker,
		ServiceTypeM3,
		ServiceTypeM3Aggregator,
		ServiceTypeOpensearch,
		ServiceTypeFlink,
	}
}

func serviceCommonSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"project": commonSchemaProjectReference,

		"cloud_name": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Defines where the cloud provider and region where the service is hosted in. This can be changed freely after service is created. Changing the value will trigger a potentially lengthy migration process for the service. Format is cloud provider name (`aws`, `azure`, `do` `google`, `upcloud`, etc.), dash, and the cloud provider specific region name. These are documented on each Cloud provider's own support articles, like [here for Google](https://cloud.google.com/compute/docs/regions-zones/) and [here for AWS](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Concepts.RegionsAndAvailabilityZones.html).",
		},
		"plan": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Defines what kind of computing resources are allocated for the service. It can be changed after creation, though there are some restrictions when going to a smaller plan such as the new plan must have sufficient amount of disk space to store all current data and switching to a plan with fewer nodes might not be supported. The basic plan names are `hobbyist`, `startup-x`, `business-x` and `premium-x` where `x` is (roughly) the amount of memory on each node (also other attributes like number of CPUs and amount of disk space varies but naming is based on memory). The available options can be seem from the [Aiven pricing page](https://aiven.io/pricing).",
		},
		"service_name": {
			Type:        schema.TypeString,
			Required:    true,
			ForceNew:    true,
			Description: "Specifies the actual name of the service. The name cannot be changed later without destroying and re-creating the service so name should be picked based on intended service usage rather than current attributes.",
		},
		"service_type": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Aiven internal service type code",
		},
		"project_vpc_id": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Specifies the VPC the service should run in. If the value is not set the service is not run inside a VPC. When set, the value should be given as a reference to set up dependencies correctly and the VPC must be in the same cloud and region as the service itself. Project can be freely moved to and from VPC after creation but doing so triggers migration to new servers so the operation can take significant amount of time to complete if the service has a lot of data.",
		},
		"maintenance_window_dow": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Day of week when maintenance operations should be performed. One monday, tuesday, wednesday, etc.",
			DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
				return new == ""
			},
		},
		"maintenance_window_time": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Time of day when maintenance operations should be performed. UTC time in HH:mm:ss format.",
			DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
				return new == ""
			},
		},
		"termination_protection": {
			Type:        schema.TypeBool,
			Optional:    true,
			Description: "Prevents the service from being deleted. It is recommended to set this to `true` for all production services to prevent unintentional service deletion. This does not shield against deleting databases or topics but for services with backups much of the content can at least be restored from backup in case accidental deletion is done.",
		},
		"disk_space": {
			Type:             schema.TypeString,
			Optional:         true,
			Description:      "The disk space of the service, possible values depend on the service type, the cloud provider and the project. Reducing will result in the service rebalancing.",
			DiffSuppressFunc: diskSpaceDiffSuppressFunc,
			ValidateFunc:     validateHumanByteSizeString,
		},
		"disk_space_default": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "The default disk space of the service, possible values depend on the service type, the cloud provider and the project. Its also the minimum value for `disk_space`",
		},
		"disk_space_step": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "The default disk space step of the service, possible values depend on the service type, the cloud provider and the project. `disk_space` needs to increment from `disk_space_default` by increments of this size.",
		},
		"disk_space_cap": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "The maximum disk space of the service, possible values depend on the service type, the cloud provider and the project.",
		},
		"service_uri": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "URI for connecting to the service. Service specific info is under \"kafka\", \"pg\", etc.",
			Sensitive:   true,
		},
		"service_host": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "The hostname of the service.",
		},
		"service_port": {
			Type:        schema.TypeInt,
			Computed:    true,
			Description: "The port of the service",
		},
		"service_password": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Password used for connecting to the service, if applicable",
			Sensitive:   true,
		},
		"service_username": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Username used for connecting to the service, if applicable",
		},
		"state": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Service state. One of `POWEROFF`, `REBALANCING`, `REBUILDING` or `RUNNING`.",
		},
		"service_integrations": {
			Type:        schema.TypeList,
			Optional:    true,
			Description: "Service integrations to specify when creating a service. Not applied after initial service creation",
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"source_service_name": {
						Type:        schema.TypeString,
						Required:    true,
						Description: "Name of the source service",
					},
					"integration_type": {
						Type:        schema.TypeString,
						Required:    true,
						Description: "Type of the service integration. The only supported value at the moment is `read_replica`",
					},
				},
			},
		},
		"components": {
			Type:        schema.TypeList,
			Computed:    true,
			Description: "Service component information objects",
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"component": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "Service component name",
					},
					"host": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "DNS name for connecting to the service component",
					},
					"kafka_authentication_method": {
						Type:        schema.TypeString,
						Computed:    true,
						Optional:    true,
						Description: "Kafka authentication method. This is a value specific to the 'kafka' service component",
					},
					"port": {
						Type:        schema.TypeInt,
						Computed:    true,
						Description: "Port number for connecting to the service component",
					},
					"route": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "Network access route",
					},
					"ssl": {
						Type:     schema.TypeBool,
						Computed: true,
						Description: "Whether the endpoint is encrypted or accepts plaintext. By default endpoints are " +
							"always encrypted and this property is only included for service components they may " +
							"disable encryption",
					},
					"usage": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "DNS usage name",
					},
				},
			},
		},
	}
}

var aivenServiceSchema = map[string]*schema.Schema{
	"project": {
		Type:        schema.TypeString,
		Required:    true,
		Description: "Target project",
		ForceNew:    true,
	},
	"cloud_name": {
		Type:        schema.TypeString,
		Optional:    true,
		Description: "Cloud the service runs in",
	},
	"plan": {
		Type:        schema.TypeString,
		Optional:    true,
		Description: "Subscription plan",
	},
	"service_name": {
		Type:        schema.TypeString,
		Required:    true,
		Description: "Service name",
		ForceNew:    true,
	},
	"service_type": {
		Type:         schema.TypeString,
		Required:     true,
		Description:  "Service type code",
		ForceNew:     true,
		ValidateFunc: validation.StringInSlice(availableServiceTypes(), false),
	},
	"project_vpc_id": {
		Type:        schema.TypeString,
		Optional:    true,
		Description: "Identifier of the VPC the service should be in, if any",
	},
	"maintenance_window_dow": {
		Type:        schema.TypeString,
		Optional:    true,
		Description: "Day of week when maintenance operations should be performed. One monday, tuesday, wednesday, etc.",
		DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
			return new == ""
		},
	},
	"maintenance_window_time": {
		Type:        schema.TypeString,
		Optional:    true,
		Description: "Time of day when maintenance operations should be performed. UTC time in HH:mm:ss format.",
		DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
			return new == ""
		},
	},
	"termination_protection": {
		Type:        schema.TypeBool,
		Optional:    true,
		Description: "Prevent service from being deleted. It is recommended to have this enabled for all services.",
	},
	"disk_space": {
		Type:             schema.TypeString,
		Optional:         true,
		Description:      "The disk space of the service, possible values depend on the service type, the cloud provider and the project. Reducing will result in the service rebalancing.",
		DiffSuppressFunc: diskSpaceDiffSuppressFunc,
		ValidateFunc:     validateHumanByteSizeString,
	},
	"disk_space_default": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "The default disk space of the service, possible values depend on the service type, the cloud provider and the project. Its also the minimum value for `disk_space`",
	},
	"disk_space_step": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "The default disk space step of the service, possible values depend on the service type, the cloud provider and the project. `disk_space` needs to increment from `disk_space_default` by increments of this size.",
	},
	"disk_space_cap": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "The maximum disk space of the service, possible values depend on the service type, the cloud provider and the project.",
	},
	"service_uri": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "URI for connecting to the service. Service specific info is under \"kafka\", \"pg\", etc.",
		Sensitive:   true,
	},
	"service_host": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "Service hostname",
	},
	"service_integrations": {
		Type:        schema.TypeList,
		Optional:    true,
		Description: "Service integrations to specify when creating a service. Not applied after initial service creation",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"source_service_name": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Name of the source service",
				},
				"integration_type": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Type of the service integration. The only supported value at the moment is 'read_replica'",
				},
			},
		},
	},
	"components": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Service component information objects",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"component": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Service component name",
				},
				"host": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "DNS name for connecting to the service component",
				},
				"kafka_authentication_method": {
					Type:        schema.TypeString,
					Computed:    true,
					Optional:    true,
					Description: "Kafka authentication method. This is a value specific to the 'kafka' service component",
				},
				"port": {
					Type:        schema.TypeInt,
					Computed:    true,
					Description: "Port number for connecting to the service component",
				},
				"route": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Network access route",
				},
				"ssl": {
					Type:     schema.TypeBool,
					Computed: true,
					Description: "Whether the endpoint is encrypted or accepts plaintext. By default endpoints are " +
						"always encrypted and this property is only included for service components they may " +
						"disable encryption",
				},
				"usage": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "DNS usage name",
				},
			},
		},
	},

	"service_port": {
		Type:        schema.TypeInt,
		Computed:    true,
		Description: "Service port",
	},
	"service_password": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "Password used for connecting to the service, if applicable",
		Sensitive:   true,
	},
	"service_username": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "Username used for connecting to the service, if applicable",
	},
	"state": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "Service state. One of `POWEROFF`, `REBALANCING`, `REBUILDING` and `RUNNING`.",
	},
	"cassandra": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Cassandra specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{},
		},
	},
	"cassandra_user_config": generateServiceUserConfiguration(ServiceTypeCassandra),
	"elasticsearch": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Elasticsearch specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"kibana_uri": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "URI for Kibana frontend",
					Sensitive:   true,
				},
			},
		},
	},
	"elasticsearch_user_config": generateServiceUserConfiguration(ServiceTypeElasticsearch),
	"opensearch": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Opensearch specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"opensearch_dashboards_uri": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "URI for Opensearch dashboard frontend",
					Sensitive:   true,
				},
			},
		},
	},
	"opensearch_user_config": generateServiceUserConfiguration(ServiceTypeOpensearch),
	"grafana": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Grafana specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{},
		},
	},
	"grafana_user_config": generateServiceUserConfiguration(ServiceTypeGrafana),
	"influxdb": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "InfluxDB specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"database_name": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Name of the default InfluxDB database",
				},
			},
		},
	},
	"influxdb_user_config": generateServiceUserConfiguration(ServiceTypeInfluxDB),
	"kafka": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Kafka specific server provided values",
		Optional:    true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"access_cert": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "The Kafka client certificate",
					Optional:    true,
					Sensitive:   true,
				},
				"access_key": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "The Kafka client certificate key",
					Optional:    true,
					Sensitive:   true,
				},
				"connect_uri": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "The Kafka Connect URI, if any",
					Optional:    true,
					Sensitive:   true,
				},
				"rest_uri": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "The Kafka REST URI, if any",
					Optional:    true,
					Sensitive:   true,
				},
				"schema_registry_uri": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "The Schema Registry URI, if any",
					Optional:    true,
					Sensitive:   true,
				},
			},
		},
	},
	"kafka_user_config": generateServiceUserConfiguration(ServiceTypeKafka),
	"kafka_connect": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Kafka Connect specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{},
		},
	},
	"kafka_connect_user_config": generateServiceUserConfiguration(ServiceTypeKafkaConnect),
	"mysql": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "MySQL specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{},
		},
	},
	"mysql_user_config": generateServiceUserConfiguration(ServiceTypeMySQL),
	"kafka_mirrormaker": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Kafka MirrorMaker 2 specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{},
		},
	},
	"kafka_mirrormaker_user_config": generateServiceUserConfiguration(ServiceTypeKafkaMirrormaker),
	"pg": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "PostgreSQL specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"replica_uri": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "PostgreSQL replica URI for services with a replica",
					Sensitive:   true,
				},
				"uri": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "PostgreSQL master connection URI",
					Optional:    true,
					Sensitive:   true,
				},
				"dbname": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Primary PostgreSQL database name",
				},
				"host": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "PostgreSQL master node host IP or name",
				},
				"password": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "PostgreSQL admin user password",
					Sensitive:   true,
				},
				"port": {
					Type:        schema.TypeInt,
					Computed:    true,
					Description: "PostgreSQL port",
				},
				"sslmode": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "PostgreSQL sslmode setting (currently always \"require\")",
				},
				"user": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "PostgreSQL admin user name",
				},
			},
		},
	},
	"pg_user_config": generateServiceUserConfiguration(ServiceTypePG),
	"redis": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Redis specific server provided values",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{},
		},
	},
	"redis_user_config": generateServiceUserConfiguration(ServiceTypeRedis),
	"flink": {
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Flink specific server provided values",
		Optional:    true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"host_ports": {
					Type:        schema.TypeList,
					Computed:    true,
					Description: "Host and Port of a Flink server",
					Optional:    true,
					Elem: &schema.Schema{
						Type: schema.TypeString,
					},
				},
			},
		},
	},
	"flink_user_config": generateServiceUserConfiguration(ServiceTypeFlink),
}

func resourceService() *schema.Resource {
	return &schema.Resource{
		Description:        "The Service resource allows the creation and management of Aiven Services.",
		DeprecationMessage: "Please use the specific service resources instead of this resource.",
		CreateContext:      resourceServiceCreateWrapper("service"),
		ReadContext:        resourceServiceRead,
		UpdateContext:      resourceServiceUpdate,
		DeleteContext:      resourceServiceDelete,
		CustomizeDiff:      resourceServiceCustomizeDiff,
		Importer: &schema.ResourceImporter{
			StateContext: resourceServiceState,
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(20 * time.Minute),
			Update: schema.DefaultTimeout(20 * time.Minute),
		},
		Schema: aivenServiceSchema,
	}
}

func resourceServiceCreateWrapper(serviceType string) schema.CreateContextFunc {
	if serviceType == "service" {
		return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
			// Need to set empty value for all services or all Terraform keeps on showing there's
			// a change in the computed values that don't match actual service type
			if err := d.Set(ServiceTypeCassandra, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeElasticsearch, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeGrafana, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeInfluxDB, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeKafka, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeKafkaConnect, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeKafkaMirrormaker, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeMySQL, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypePG, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeRedis, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeOpensearch, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set(ServiceTypeFlink, []map[string]interface{}{}); err != nil {
				return diag.FromErr(err)
			}
			return resourceServiceCreate(ctx, d, m)
		}
	}

	return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		if err := d.Set("service_type", serviceType); err != nil {
			return diag.FromErr(err)
		}
		if err := d.Set(serviceType, []map[string]interface{}{}); err != nil {
			return diag.FromErr(err)
		}

		return resourceServiceCreate(ctx, d, m)
	}
}

func resourceServiceCustomizeDiffWrapper(serviceType string) schema.CustomizeDiffFunc {
	return func(ctx context.Context, d *schema.ResourceDiff, m interface{}) error {
		if err := d.SetNew("service_type", serviceType); err != nil {
			return err
		}
		return resourceServiceCustomizeDiff(ctx, d, m)
	}
}

func resourceServiceRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	projectName, serviceName := splitResourceID2(d.Id())
	service, err := client.Services.Get(projectName, serviceName)
	if err != nil {
		if err = resourceReadHandleNotFound(err, d); err != nil {
			return diag.FromErr(fmt.Errorf("unable to GET service %s: %s", d.Id(), err))
		}
		return nil
	}
	servicePlanParams, err := resourceServiceGetServicePlanParameters(ctx, client, d)
	if err != nil {
		return diag.FromErr(fmt.Errorf("unable to get service plan parameters: %w", err))
	}

	err = copyServicePropertiesFromAPIResponseToTerraform(d, service, servicePlanParams, projectName)
	if err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceServiceCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	// get service plan specific defaults
	servicePlanParams, err := resourceServiceGetServicePlanParameters(ctx, client, d)
	if err != nil {
		return diag.FromErr(fmt.Errorf("unable to get service plan parameters: %w", err))
	}

	serviceType := d.Get("service_type").(string)
	project := d.Get("project").(string)

	if _, err = client.Services.Create(
		project,
		aiven.CreateServiceRequest{
			Cloud:                 d.Get("cloud_name").(string),
			Plan:                  d.Get("plan").(string),
			ProjectVPCID:          resourceServiceGetProjectVPCIdPointer(d),
			ServiceIntegrations:   resourceServiceGetAPIServiceIntegrations(d),
			MaintenanceWindow:     resourceServiceGetMaintenanceWindow(d),
			ServiceName:           d.Get("service_name").(string),
			ServiceType:           serviceType,
			TerminationProtection: d.Get("termination_protection").(bool),
			DiskSpaceMB:           resourceServiceGetDiskSpaceMBOrServicePlanDefault(d, servicePlanParams),
			UserConfig:            ConvertTerraformUserConfigToAPICompatibleFormat("service", serviceType, true, d),
		},
	); err != nil {
		return diag.FromErr(err)
	}

	service, err := resourceServiceWait(ctx, d, m, "create")
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(buildResourceID(project, service.Name))

	return resourceServiceRead(ctx, d, m)
}

func resourceServiceUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	// get service plan specific defaults
	servicePlanParams, err := resourceServiceGetServicePlanParameters(ctx, client, d)
	if err != nil {
		return diag.FromErr(fmt.Errorf("unable to get service plan parameters: %w", err))
	}

	projectName, serviceName := splitResourceID2(d.Id())

	if _, err := client.Services.Update(
		projectName,
		serviceName,
		aiven.UpdateServiceRequest{
			Cloud:                 d.Get("cloud_name").(string),
			Plan:                  d.Get("plan").(string),
			MaintenanceWindow:     resourceServiceGetMaintenanceWindow(d),
			ProjectVPCID:          resourceServiceGetProjectVPCIdPointer(d),
			Powered:               true,
			TerminationProtection: d.Get("termination_protection").(bool),
			DiskSpaceMB:           resourceServiceGetDiskSpaceMBOrServicePlanDefault(d, servicePlanParams),
			UserConfig:            ConvertTerraformUserConfigToAPICompatibleFormat("service", d.Get("service_type").(string), false, d),
		},
	); err != nil {
		return diag.FromErr(err)
	}

	if _, err := resourceServiceWait(ctx, d, m, "update"); err != nil {
		return diag.FromErr(err)
	}

	return resourceServiceRead(ctx, d, m)
}

func resourceServiceDelete(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	projectName, serviceName := splitResourceID2(d.Id())

	err := client.Services.Delete(projectName, serviceName)
	if err != nil && !aiven.IsNotFound(err) {
		return diag.FromErr(err)
	}

	return nil
}

func resourceServiceState(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
	client := m.(*aiven.Client)

	if len(strings.Split(d.Id(), "/")) != 2 {
		return nil, fmt.Errorf("invalid identifier %v, expected <project_name>/<service_name>", d.Id())
	}

	projectName, serviceName := splitResourceID2(d.Id())
	service, err := client.Services.Get(projectName, serviceName)
	if err != nil {
		return nil, fmt.Errorf("unable to GET service %s: %s", d.Id(), err)
	}
	servicePlanParams, err := resourceServiceGetServicePlanParameters(ctx, client, d)
	if err != nil {
		return nil, fmt.Errorf("unable to get service plan parameters: %w", err)
	}

	err = copyServicePropertiesFromAPIResponseToTerraform(d, service, servicePlanParams, projectName)
	if err != nil {
		return nil, err
	}

	return []*schema.ResourceData{d}, nil
}

func resourceServiceWait(ctx context.Context, d *schema.ResourceData, m interface{}, operation string) (*aiven.Service, error) {
	var timeout time.Duration
	if operation == "create" {
		timeout = d.Timeout(schema.TimeoutCreate)
	} else {
		timeout = d.Timeout(schema.TimeoutUpdate)
	}

	w := &ServiceChangeWaiter{
		Client:      m.(*aiven.Client),
		Operation:   operation,
		Project:     d.Get("project").(string),
		ServiceName: d.Get("service_name").(string),
	}

	service, err := w.Conf(timeout).WaitForStateContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("error waiting for Aiven service to be RUNNING: %s", err)
	}

	return service.(*aiven.Service), nil
}

func copyServicePropertiesFromAPIResponseToTerraform(
	d *schema.ResourceData,
	service *aiven.Service,
	servicePlanParams servicePlanParameters,
	project string,
) error {
	serviceType := d.Get("service_type").(string)
	if _, ok := d.GetOk("service_type"); !ok {
		serviceType = service.Type
	}

	if err := d.Set("cloud_name", service.CloudName); err != nil {
		return err
	}
	if err := d.Set("service_name", service.Name); err != nil {
		return err
	}
	if err := d.Set("state", service.State); err != nil {
		return err
	}
	if err := d.Set("plan", service.Plan); err != nil {
		return err
	}
	if err := d.Set("service_type", serviceType); err != nil {
		return err
	}
	if err := d.Set("termination_protection", service.TerminationProtection); err != nil {
		return err
	}
	if err := d.Set("maintenance_window_dow", service.MaintenanceWindow.DayOfWeek); err != nil {
		return err
	}
	if err := d.Set("maintenance_window_time", service.MaintenanceWindow.TimeOfDay); err != nil {
		return err
	}
	if err := d.Set("disk_space", humanReadableByteSize(service.DiskSpaceMB*units.MiB)); err != nil {
		return err
	}
	if err := d.Set("disk_space_default", humanReadableByteSize(servicePlanParams.diskSizeMBDefault*units.MiB)); err != nil {
		return err
	}
	if err := d.Set("disk_space_step", humanReadableByteSize(servicePlanParams.diskSizeMBStep*units.MiB)); err != nil {
		return err
	}
	if err := d.Set("disk_space_cap", humanReadableByteSize(servicePlanParams.diskSizeMBMax*units.MiB)); err != nil {
		return err
	}
	if err := d.Set("service_uri", service.URI); err != nil {
		return err
	}
	if err := d.Set("project", project); err != nil {
		return err
	}

	if service.ProjectVPCID != nil {
		if err := d.Set("project_vpc_id", buildResourceID(project, *service.ProjectVPCID)); err != nil {
			return err
		}
	}
	userConfig := ConvertAPIUserConfigToTerraformCompatibleFormat(
		"service", serviceType, service.UserConfig)
	if err := d.Set(serviceType+"_user_config",
		ipfilter.Normalize(d.Get(serviceType+"_user_config"), userConfig)); err != nil {
		return fmt.Errorf("cannot set `%s_user_config` : %s;"+
			"Please make sure that all Aiven services have unique service names", serviceType, err)
	}

	params := service.URIParams
	if err := d.Set("service_host", params["host"]); err != nil {
		return err
	}

	port, _ := strconv.ParseInt(params["port"], 10, 32)
	if err := d.Set("service_port", port); err != nil {
		return err
	}

	password, passwordOK := params["password"]
	username, usernameOK := params["user"]
	if passwordOK {
		if err := d.Set("service_password", password); err != nil {
			return err
		}
	}
	if usernameOK {
		if err := d.Set("service_username", username); err != nil {
			return err
		}
	}

	if err := d.Set("components", flattenServiceComponents(service)); err != nil {
		return fmt.Errorf("cannot set `components` : %s", err)
	}

	return copyConnectionInfoFromAPIResponseToTerraform(d, serviceType, service.ConnectionInfo)
}

func flattenServiceComponents(r *aiven.Service) []map[string]interface{} {
	var components []map[string]interface{}

	for _, c := range r.Components {
		component := map[string]interface{}{
			"component": c.Component,
			"host":      c.Host,
			"port":      c.Port,
			"route":     c.Route,
			"usage":     c.Usage,
		}
		components = append(components, component)
	}

	return components
}

func copyConnectionInfoFromAPIResponseToTerraform(
	d *schema.ResourceData,
	serviceType string,
	connectionInfo aiven.ConnectionInfo,
) error {
	props := make(map[string]interface{})

	switch serviceType {
	case "cassandra":
	case "opensearch":
		props["opensearch_dashboards_uri"] = connectionInfo.OpensearchDashboardsURI
	case "elasticsearch":
		props["kibana_uri"] = connectionInfo.KibanaURI
	case "grafana":
	case "influxdb":
		props["database_name"] = connectionInfo.InfluxDBDatabaseName
	case "kafka":
		props["access_cert"] = connectionInfo.KafkaAccessCert
		props["access_key"] = connectionInfo.KafkaAccessKey
		props["connect_uri"] = connectionInfo.KafkaConnectURI
		props["rest_uri"] = connectionInfo.KafkaRestURI
		props["schema_registry_uri"] = connectionInfo.SchemaRegistryURI
	case "kafka_connect":
	case "mysql":
	case "pg":
		if connectionInfo.PostgresURIs != nil && len(connectionInfo.PostgresURIs) > 0 {
			props["uri"] = connectionInfo.PostgresURIs[0]
		}
		if connectionInfo.PostgresParams != nil && len(connectionInfo.PostgresParams) > 0 {
			params := connectionInfo.PostgresParams[0]
			props["dbname"] = params.DatabaseName
			props["host"] = params.Host
			props["password"] = params.Password
			port, err := strconv.ParseInt(params.Port, 10, 32)
			if err == nil {
				props["port"] = int(port)
			}
			props["sslmode"] = params.SSLMode
			props["user"] = params.User
		}
		props["replica_uri"] = connectionInfo.PostgresReplicaURI
	case "redis":
	case "flink":
		props["host_ports"] = connectionInfo.FlinkHostPorts
	case "kafka_mirrormaker":
	case "m3db":
	case "m3aggregator":
	default:
		panic(fmt.Sprintf("Unsupported service type %v", serviceType))
	}

	if err := d.Set(serviceType, []map[string]interface{}{props}); err != nil {
		return err
	}

	return nil
}

func resourceServiceGetAPIServiceIntegrations(d resourceStateOrResourceDiff) []aiven.NewServiceIntegration {
	var apiServiceIntegrations []aiven.NewServiceIntegration
	tfServiceIntegrations := d.Get("service_integrations")
	if tfServiceIntegrations != nil {
		tfServiceIntegrationList := tfServiceIntegrations.([]interface{})
		for _, definition := range tfServiceIntegrationList {
			definitionMap := definition.(map[string]interface{})
			sourceService := definitionMap["source_service_name"].(string)
			apiIntegration := aiven.NewServiceIntegration{
				IntegrationType: definitionMap["integration_type"].(string),
				SourceService:   &sourceService,
				UserConfig:      make(map[string]interface{}),
			}
			apiServiceIntegrations = append(apiServiceIntegrations, apiIntegration)
		}
	}
	return apiServiceIntegrations
}

func resourceServiceGetProjectVPCIdPointer(d resourceStateOrResourceDiff) *string {
	vpcID := d.Get("project_vpc_id").(string)
	var vpcIDPointer *string
	if len(vpcID) > 0 {
		_, vpcID := splitResourceID2(vpcID)
		vpcIDPointer = &vpcID
	}
	return vpcIDPointer
}

func resourceServiceGetMaintenanceWindow(d resourceStateOrResourceDiff) *aiven.MaintenanceWindow {
	dow := d.Get("maintenance_window_dow").(string)
	t := d.Get("maintenance_window_time").(string)
	if len(dow) > 0 && len(t) > 0 {
		return &aiven.MaintenanceWindow{DayOfWeek: dow, TimeOfDay: t}
	}
	return nil
}

func resourceServiceGetDiskSpaceMBOrServicePlanDefault(d resourceStateOrResourceDiff, params servicePlanParameters) int {
	diskSpaceSchema, ok := d.GetOk("disk_space")
	if !ok {
		return params.diskSizeMBDefault
	}
	diskSizeMB, _ := units.RAMInBytes(diskSpaceSchema.(string))
	return int(diskSizeMB / units.MiB)
}

type servicePlanParameters struct {
	diskSizeMBDefault int
	diskSizeMBStep    int
	diskSizeMBMax     int
}

func servicePlanParamsDiskSizeIsConfigurable(params servicePlanParameters) bool {
	return params.diskSizeMBMax != 0 && params.diskSizeMBStep != 0
}

func servicePlanParamsDiskSizeMatchesDefault(params servicePlanParameters, requestedDiskSpaceMB int) bool {
	return params.diskSizeMBDefault == requestedDiskSpaceMB
}

func servicePlanParamsCheckDiskSize(params servicePlanParameters, requestedDiskSizeMB int) error {
	if !servicePlanParamsDiskSizeMatchesDefault(params, requestedDiskSizeMB) && !servicePlanParamsDiskSizeIsConfigurable(params) {
		return fmt.Errorf("service plan parameters are not configurable for this service")
	}

	humanReadableDiskSpaceDefault := humanReadableByteSize(params.diskSizeMBDefault * units.MiB)
	humanReadableDiskSpaceMax := humanReadableByteSize(params.diskSizeMBMax * units.MiB)
	humanReadableDiskSpaceStep := humanReadableByteSize(params.diskSizeMBStep * units.MiB)
	humanReadableRequestedDiskSpace := humanReadableByteSize(requestedDiskSizeMB * units.MiB)

	if requestedDiskSizeMB < params.diskSizeMBDefault {
		return fmt.Errorf("requested disk size is too small: '%s' < '%s'", humanReadableRequestedDiskSpace, humanReadableDiskSpaceDefault)
	}
	if params.diskSizeMBMax != 0 {
		if requestedDiskSizeMB > params.diskSizeMBMax {
			return fmt.Errorf("requested disk size is too large: '%s' > '%s'", humanReadableRequestedDiskSpace, humanReadableDiskSpaceMax)
		}
	}
	if params.diskSizeMBStep != 0 {
		if (requestedDiskSizeMB-params.diskSizeMBDefault)%params.diskSizeMBStep != 0 {
			return fmt.Errorf("requested disk size has to increase from: '%s' in increments of '%s'", humanReadableDiskSpaceDefault, humanReadableDiskSpaceStep)
		}
	}
	return nil
}

func resourceServiceGetServicePlanParameters(ctx context.Context, client *aiven.Client, d resourceStateOrResourceDiff) (servicePlanParameters, error) {
	project := d.Get("project").(string)
	serviceType := d.Get("service_type").(string)
	servicePlan := d.Get("plan").(string)
	serviceRegion := d.Get("cloud_name").(string)

	serviceTypesResponse, err := client.ServiceTypes.Get(project)
	if err != nil {
		return servicePlanParameters{}, fmt.Errorf("unable to get service types from api: %w", err)
	}
	serviceTypeDescription, ok := serviceTypesResponse.ServiceTypes[serviceType]
	if !ok {
		return servicePlanParameters{}, fmt.Errorf("service type '%s' unknown to service types api", serviceType)
	}
	for i := range serviceTypeDescription.ServicePlans {
		servicePlanDescription := serviceTypeDescription.ServicePlans[i]
		if servicePlanDescription.ServicePlan != servicePlan {
			// wrong service plan
			continue
		}
		serviceRegionDescription, ok := servicePlanDescription.Regions[serviceRegion]
		if !ok {
			// the region not be admissible for the plan
			continue
		}
		return servicePlanParameters{
			diskSizeMBDefault: serviceRegionDescription.DiskSpaceMB,
			diskSizeMBMax:     serviceRegionDescription.DiskSpaceCapMB,
			diskSizeMBStep:    serviceRegionDescription.DiskSpaceStepMB,
		}, nil
	}
	return servicePlanParameters{}, fmt.Errorf("unable to find service plan parameters for '%s', '%s', '%s'", serviceType, servicePlan, serviceRegion)
}

// resourceServiceCustomizeDiff validates that the diff has a chance to be applied
func resourceServiceCustomizeDiff(ctx context.Context, d *schema.ResourceDiff, m interface{}) error {
	// Some service integrations can be created when creating a service, think 'read_replica' for example.
	// Those are only applicable for the initial request and should not
	if err := resourceServiceCustomizeDiffCheckNoNewServiceIntegrationAfterCreation(ctx, d, m); err != nil {
		return fmt.Errorf("failed service integration check: %w", err)
	}
	// We need to validate that the configured disk space matches the service plan; consider the situation
	// that we have custom disk space configured but want to migrate into a cloud and plan that do not support the configured
	// disk space anymore
	if err := resourceServiceCustomizeDiffCheckDiskSpace(ctx, d, m); err != nil {
		return fmt.Errorf("failed disk space check: %w", err)
	}

	return nil
}

func resourceServiceCustomizeDiffCheckNoNewServiceIntegrationAfterCreation(ctx context.Context, d *schema.ResourceDiff, m interface{}) error {
	if len(d.Id()) > 0 && d.HasChange("service_integrations") && len(d.Get("service_integrations").([]interface{})) != 0 {
		return fmt.Errorf("service_integrations field can only be set during creation of a service")
	}
	return nil
}

func resourceServiceCustomizeDiffCheckDiskSpace(ctx context.Context, d *schema.ResourceDiff, m interface{}) error {
	client := m.(*aiven.Client)

	// the default should always be OK
	if diskSizeSchema, ok := d.GetOk("disk_space"); !ok || diskSizeSchema.(string) == "" {
		return nil
	}

	// check that disk size matches plan
	servicePlanParams, err := resourceServiceGetServicePlanParameters(ctx, client, d)
	if err != nil {
		return fmt.Errorf("unable to get service plan parameters: %w", err)
	}

	if err = servicePlanParamsCheckDiskSize(servicePlanParams, resourceServiceGetDiskSpaceMBOrServicePlanDefault(d, servicePlanParams)); err != nil {
		return fmt.Errorf("disk size check failed: %w", err)
	}
	return nil
}

func humanReadableByteSize(bsize int) string {
	// we only allow GiB as unit and show decimal places to MiB precision, this should fix rounding issues
	// when converting from mb to human readable and back
	var suffixes = []string{"B", "KiB", "MiB", "GiB"}

	return units.CustomSize("%.12g%s", float64(bsize), 1024.0, suffixes)
}
