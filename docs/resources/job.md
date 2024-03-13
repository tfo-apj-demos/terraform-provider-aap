---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "aap_job Resource - terraform-provider-aap"
subcategory: ""
description: |-
  
---

# aap_job (Resource)





<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `job_template_id` (Number) Id of the job template.

### Optional

- `extra_vars` (String) Extra Variables. Must be provided as either a JSON or YAML string.
- `inventory_id` (Number) Identifier for the inventory where job should be created in. If not provided, the job will be created in the default inventory.
- `triggers` (Map of String) Map of arbitrary keys and values that, when changed, will trigger a creation of a new Job on AAP. Use 'terraform taint' if you want to force the creation of a new job without changing this value.

### Read-Only

- `ignored_fields` (List of String) The list of properties set by the user but ignored on server side.
- `job_type` (String) Job type
- `status` (String) Status of the job
- `url` (String) URL of the job template