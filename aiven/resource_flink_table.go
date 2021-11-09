// Copyright (c) 2017 jelmersnoeck
// Copyright (c) 2018-2021 Aiven, Helsinki, Finland. https://aiven.io/
package aiven

import (
	"context"

	"github.com/aiven/aiven-go-client"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var (
	aivenFlinkTableConnectorTypes    = []string{"kafka", "upsert_kafka"}
	aivenFlinkTableKafkaValueFormats = []string{"avro", "avro-confluent", "debezium-avro-confluent", "debezium-json", "json"}
	aivenFlinkTableKafkaKeyFormats   = aivenFlinkTableKafkaValueFormats
)

var aivenFlinkTableSchema = map[string]*schema.Schema{
	"project":      commonSchemaProjectReference,
	"service_name": commonSchemaServiceNameReference,

	"table_name": {
		Type:        schema.TypeString,
		Required:    true,
		ForceNew:    true,
		Description: complex("Specifies the name of the table.").forceNew().build(),
	},
	"integration_id": {
		Type:        schema.TypeString,
		Required:    true,
		ForceNew:    true,
		Description: complex("The id of the service integration that is used with this table. It must have the service integration type `flink`.").referenced().forceNew().build(),
	},
	"jdbc_table": {
		Type:        schema.TypeString,
		Optional:    true,
		ForceNew:    true,
		Description: complex("Name of the jdbc table that is to be connected to this table. Valid if the service integration id refers to a mysql or postgres service.").forceNew().build(),
	},
	"connector_type": {
		Type:         schema.TypeString,
		Optional:     true,
		ForceNew:     true,
		Description:  complex("When used as a source, upsert Kafka connectors update values that use an existing key and delete values that are null. For sinks, the connector correspondingly writes update or delete messages in a compacted topic. If no matching key is found, the values are added as new entries. For more information, see the Apache Flink documentation").forceNew().possibleValues(stringSliceToInterfaceSlice(aivenFlinkTableConnectorTypes)...).build(),
		ValidateFunc: validateStringEnum(aivenFlinkTableConnectorTypes...),
	},
	"kafka_topic": {
		Type:        schema.TypeString,
		Optional:    true,
		ForceNew:    true,
		Description: complex("Name of the kafka topic that is to be connected to this table. Valid if the service integration id refers to a kafka service.").forceNew().build(),
	},
	"kafka_key_format": {
		Type:         schema.TypeString,
		Optional:     true,
		ForceNew:     true,
		Description:  complex("Kafka Key Format").forceNew().possibleValues(stringSliceToInterfaceSlice(aivenFlinkTableKafkaKeyFormats)...).build(),
		ValidateFunc: validateStringEnum(aivenFlinkTableKafkaKeyFormats...),
	},
	"kafka_value_format": {
		Type:         schema.TypeString,
		Optional:     true,
		ForceNew:     true,
		Description:  complex("Kafka Value Format").forceNew().possibleValues(stringSliceToInterfaceSlice(aivenFlinkTableKafkaValueFormats)...).build(),
		ValidateFunc: validateStringEnum(aivenFlinkTableKafkaValueFormats...),
	},
	"like_options": {
		Type:        schema.TypeString,
		Optional:    true,
		ForceNew:    true,
		Description: complex("[LIKE](https://nightlies.apache.org/flink/flink-docs-master/docs/dev/table/sql/create/#like) statement for table creation.").forceNew().build(),
	},
	"partitioned_by": {
		Type:        schema.TypeString,
		Optional:    true,
		ForceNew:    true,
		Description: complex("A column from the `schema_sql` field to partition this table by.").forceNew().build(),
	},
	"schema_sql": {
		Type:        schema.TypeString,
		Required:    true,
		ForceNew:    true,
		Description: complex("The SQL statement to create the table.").forceNew().build(),
	},
	"table_id": {
		Type:        schema.TypeString,
		Computed:    true,
		Description: "The Table ID of the flink table in the flink service.",
	},
}

func resourceFlinkTable() *schema.Resource {
	return &schema.Resource{
		Description:   "The Flink Table resource allows the creation and management of Aiven Tables.",
		CreateContext: resourceFlinkTableCreate,
		ReadContext:   resourceFlinkTableRead,
		DeleteContext: resourceFlinkTableDelete,
		Schema:        aivenFlinkTableSchema,
	}
}

func resourceFlinkTableRead(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	project, serviceName, tableId := splitResourceID3(d.Id())

	r, err := client.FlinkTables.Get(project, serviceName, aiven.GetFlinkTableRequest{TableId: tableId})
	if err != nil {
		return diag.FromErr(resourceReadHandleNotFound(err, d))
	}

	if err := d.Set("project", project); err != nil {
		return diag.Errorf("error setting Flink Tables `project` for resource %s: %s", d.Id(), err)
	}
	if err := d.Set("service_name", serviceName); err != nil {
		return diag.Errorf("error setting Flink Tables `service_name` for resource %s: %s", d.Id(), err)
	}
	if err := d.Set("integration_id", r.IntegrationId); err != nil {
		return diag.Errorf("error setting Flink Tables `integration_id` for resource %s: %s", d.Id(), err)
	}
	if err := d.Set("table_id", r.TableId); err != nil {
		return diag.Errorf("error setting Flink Tables `table_id` for resource %s: %s", d.Id(), err)
	}
	if err := d.Set("table_name", r.TableName); err != nil {
		return diag.Errorf("error setting Flink Tables `table_name` for resource %s: %s", d.Id(), err)
	}

	return nil
}

func resourceFlinkTableCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	project := d.Get("project").(string)
	serviceName := d.Get("service_name").(string)
	integrationId := d.Get("integration_id").(string)
	jdbcTable := d.Get("jdbc_table").(string)
	connectorType := d.Get("connector_type").(string)
	kafkaTopic := d.Get("kafka_topic").(string)
	kafkaKeyFormat := d.Get("kafka_key_format").(string)
	kafkaValueFormat := d.Get("kafka_value_format").(string)
	likeOptions := d.Get("like_options").(string)
	tableName := d.Get("table_name").(string)
	partitionedBy := d.Get("partitioned_by").(string)
	schemaSQL := d.Get("schema_sql").(string)

	createRequest := aiven.CreateFlinkTableRequest{
		IntegrationId:    integrationId,
		JDBCTable:        jdbcTable,
		ConnectorType:    connectorType,
		KafkaTopic:       kafkaTopic,
		KafkaKeyFormat:   kafkaKeyFormat,
		KafkaValueFormat: kafkaValueFormat,
		LikeOptions:      likeOptions,
		Name:             tableName,
		PartitionedBy:    partitionedBy,
		SchemaSQL:        schemaSQL,
	}

	r, err := client.FlinkTables.Create(project, serviceName, createRequest)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(buildResourceID(project, serviceName, r.TableId))

	return resourceFlinkTableRead(ctx, d, m)
}

func resourceFlinkTableDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*aiven.Client)

	project, serviceName, tableId := splitResourceID3(d.Id())

	err := client.FlinkTables.Delete(
		project,
		serviceName,
		aiven.DeleteFlinkTableRequest{
			TableId: tableId,
		})
	if err != nil && !aiven.IsNotFound(err) {
		return diag.FromErr(err)
	}
	return nil
}
