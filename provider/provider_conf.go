// Copyright 2021, Shoreline Software Inc.
// SPDX-License-Identifier: Apache-2.0

package provider

var ObjectConfigJsonStr = `
{
	"action": {
		"attributes": {
			"type":                    { "type": "string",     "computed": true, "value": "ACTION" },
			"name":                    { "type": "label",      "required": true, "forcenew": true, "skip": true },
			"command":                 { "type": "command",    "required": true, "primary": true, "refs": {"action":1} },
			"description":             { "type": "string",     "optional": true },
			"enabled":                 { "type": "intbool",    "optional": true, "default": false },
			"params":                  { "type": "string[]",   "optional": true },
			"resource_tags_to_export": { "type": "string_set", "optional": true },
			"res_env_var":             { "type": "string",     "optional": true },
			"resource_query":          { "type": "command",    "optional": true },
			"shell":                   { "type": "string",     "optional": true },
			"timeout":                 { "type": "int",        "optional": true, "default": 60000 },
			"file_deps":               { "type": "string_set", "optional": true, "refs": {"file":1} },
			"start_short_template":    { "type": "string",     "optional": true, "step": "start_step_class.short_template" },
			"start_long_template":     { "type": "string",     "optional": true, "step": "start_step_class.long_template" },
			"start_title_template":    { "type": "string",     "optional": true, "step": "start_step_class.title_template", "suppress_null_regex": "^started \\w*$" },
			"error_short_template":    { "type": "string",     "optional": true, "step": "error_step_class.short_template" },
			"error_long_template":     { "type": "string",     "optional": true, "step": "error_step_class.long_template" },
			"error_title_template":    { "type": "string",     "optional": true, "step": "error_step_class.title_template", "suppress_null_regex": "^failed \\w*$" },
			"complete_short_template": { "type": "string",     "optional": true, "step": "complete_step_class.short_template" },
			"complete_long_template":  { "type": "string",     "optional": true, "step": "complete_step_class.long_template" },
			"complete_title_template": { "type": "string",     "optional": true, "step": "complete_step_class.title_template", "suppress_null_regex": "^completed \\w*$" },
			"allowed_entities":        { "type": "string_set", "optional": true },
			"allowed_resources_query": { "type": "command",    "optional": true },
			"communication_workspace": { "type": "string", 	   "optional": true, "min_ver": "14.1.0", "step": "communication_workspace"},
			"communication_channel":   { "type": "string", 	   "optional": true, "min_ver": "14.1.0", "step": "communication_channel"}
		}
	},

	"alarm": {
		"attributes": {
			"type":                   { "type": "string",   "computed": true, "value": "ALARM" },
			"name":                   { "type": "label",    "required": true, "forcenew": true, "skip": true },
			"fire_query":             { "type": "command",  "required": true, "primary": true, "refs": {"action":1} },
			"clear_query":            { "type": "command",  "optional": true, "refs": {"action":1} },
			"description":            { "type": "string",   "optional": true },
			"resource_query":         { "type": "command",  "optional": true },
			"enabled":                { "type": "intbool",  "optional": true, "default": false },
			"mute_query":             { "type": "string",   "optional": true },
			"resolve_short_template": { "type": "string",   "optional": true, "step": "clear_step_class.short_template" },
			"resolve_long_template":  { "type": "string",   "optional": true, "step": "clear_step_class.long_template" },
			"resolve_title_template": { "type": "string",   "optional": true, "step": "clear_step_class.title_template", "suppress_null_regex": "^cleared \\w*$" },
			"fire_short_template":    { "type": "string",   "optional": true, "step": "fire_step_class.short_template" },
			"fire_long_template":     { "type": "string",   "optional": true, "step": "fire_step_class.long_template" },
			"fire_title_template":    { "type": "string",   "optional": true, "step": "fire_step_class.title_template", "suppress_null_regex": "^fired \\w*$" },
			"condition_type":         { "type": "command",  "optional": true, "step": "condition_details.[0].condition_type" },
			"condition_value":        { "type": "string",   "optional": true, "step": "condition_details.[0].condition_value", "match_null": "0", "outtype": "float" },
			"metric_name":            { "type": "string",   "optional": true, "step": "condition_details.[0].metric_name" },
			"raise_for":              { "type": "command",  "optional": true, "step": "condition_details.[0].raise_for", "default": "local" },
			"check_interval_sec":     { "type": "command",  "optional": true, "step": "check_interval_sec", "default": 1, "outtype": "int" },
			"compile_eligible":       { "type": "bool",     "optional": true, "step": "compile_eligible", "default": true },
			"resource_type":          { "type": "resource", "optional": true, "step": "resource_type" },
			"family":                 { "type": "command",  "optional": true, "step": "config_data.family", "default": "custom" }
		}
	},

	"bot": {
		"attributes": {
			"type":                    { "type": "string",  "computed": true, "value": "BOT" },
			"name":                    { "type": "label",   "required": true, "forcenew": true, "skip": true },
			"command":                 { "type": "command", "required": true, "primary": true, "refs": {"action":1, "alarm":1},
				"compound_in": "^\\s*if\\s*(?P<alarm_statement>.*?)\\s*then\\s*(?P<action_statement>.*?)\\s*fi\\s*$",
				"compound_out": "if ${alarm_statement} then ${action_statement} fi"
			},
			"description":             { "type": "string",  "optional": true },
			"enabled":                 { "type": "intbool", "optional": true, "default": false },
			"family":                  { "type": "command", "optional": true, "step": "config_data.family", "default": "custom" },
			"action_statement":        { "type": "command", "internal": true },
			"alarm_statement":         { "type": "command", "internal": true },
			"event_type":              { "type": "string",  "optional": true, "step": "event_type", "alias": "trigger_source", "match_null": "shoreline" },
			"monitor_id":              { "type": "string",  "optional": true, "step": "monitor_id", "alias": "external_trigger_id" },
			"alarm_resource_query":    { "type": "command", "optional": true },
			"#trigger_source":         { "type": "string",  "optional": true, "preferred_alias": "event_type", "step": "trigger_source", "default": "shoreline" },
			"#external_trigger_id":    { "type": "string",  "optional": true, "preferred_alias": "monitor_id", "step": "external_trigger_id", "default": "" },
			"communication_workspace": { "type": "string", 	"optional": true, "min_ver": "14.1.0", "step": "communication_workspace"},
			"communication_channel":   { "type": "string", 	"optional": true, "min_ver": "14.1.0", "step": "communication_channel"}
		}
	},

	"circuit_breaker": {
		"attributes": {
			"type":                    { "type": "string",  "computed": true, "value": "CIRCUIT_BREAKER" },
			"name":                    { "type": "label",   "required": true, "forcenew": true, "skip": true },
			"command":                 { "type": "command", "required": true, "primary": true, "forcenew": true, "refs": {"action":1},
				"compound_in": "^\\s*(?P<resource_query>.+)\\s*\\|\\s*(?P<action_name>[a-zA-Z_][a-zA-Z_]*)\\s*$",
				"compound_out": "${resource_query} | ${action_name}"
			},
			"breaker_type":            { "type": "string",  "optional": true },
			"hard_limit":              { "type": "int",     "required": true },
			"soft_limit":              { "type": "int",     "optional": true, "default": -1 },
			"duration":                { "type": "time_s",  "required": true },
			"fail_over":               { "type": "string",  "optional": true },
			"enabled":                 { "type": "bool",    "optional": true, "default": false },
			"action_name":             { "type": "command", "internal": true },
			"resource_query":          { "type": "command", "internal": true },
			"communication_workspace": { "type": "string",  "optional": true, "min_ver": "14.1.0", "step": "communication_workspace"},
			"communication_channel":   { "type": "string",  "optional": true, "min_ver": "14.1.0", "step": "communication_channel"}
		}
	},

	"file": {
		"attributes": {
			"type":             { "type": "string",   "computed": true, "value": "FILE" },
			"name":             { "type": "label",    "required": true, "forcenew": true, "skip": true },
			"destination_path": { "type": "string",   "required": true, "primary": true },
			"description":      { "type": "string",   "optional": true },
			"resource_query":   { "type": "string",   "required": true },
			"enabled":          { "type": "intbool",  "optional": true, "default": false },
			"input_file":       { "type": "string",   "required": true, "skip": true, "not_stored": true },
			"file_data":        { "type": "string",   "computed": true, "outtype": "file" },
			"file_length":      { "type": "int",      "computed": true },
			"checksum":         { "type": "string",   "computed": true },
			"md5":              { "type": "string",   "optional": true, "proxy": "file_length,checksum,file_data" }
		}
	},

	"integration": {
		"attributes": {
			"type":                        { "type": "string",   "computed": true, "value": "INTEGRATION" },
			"name":                        { "type": "label",    "required": true, "forcenew": true, "skip": true },
			"service_name":                { "type": "command",  "required": true, "primary": true, "forcenew": true, "skip": true },
			"serial_number":               { "type": "string",   "required": true },
			"permissions_user":            { "type": "string",   "optional": true, "match_null": "Shoreline" },
			"api_key":                     { "type": "string",   "optional": true, "step": "params_unpack.api_key" },
			"app_key":                     { "type": "string",   "optional": true, "step": "params_unpack.app_key" },
			"dashboard_name":              { "type": "string",   "optional": true, "step": "params_unpack.dashboard_name" },
			"webhook_name":                { "type": "string",   "optional": true, "step": "params_unpack.webhook_name" },
			"##description":               { "type": "string",   "optional": true },
			"##account_id":                  { "type": "string",   "optional": true },
			"##insights_collector_url":      { "type": "string",   "required": true },
			"##insights_collector_api_key":  { "type": "string",   "required": true },
			"##incident_management_url":     { "type": "string",   "optional": true },
			"##incident_management_api_key": { "type": "string",   "optional": true },
			"enabled":                     { "type": "intbool",  "optional": true, "default": false }
		}
	},

	"metric": {
		"attributes": {
			"type":           { "type": "string",   "computed": true, "value": "METRIC" },
			"name":           { "type": "label",    "required": true, "forcenew": true, "skip": true },
			"value":          { "type": "command",  "required": true, "primary": true, "alias_out": "val" },
			"description":    { "type": "string",   "optional": true },
			"units":          { "type": "string",   "optional": true },
			"resource_type":  { "type": "resource", "optional": true }
		}
	},

	"notebook": {
		"attributes": {
			"type":                    { "type": "string",     "computed": true, "value": "NOTEBOOK" },
			"name":                    { "type": "label",      "required": true, "forcenew": true, "skip": true },
			"data":                    { "type": "b64json",    "required": true, "step": ".", "primary": true,
				                           "omit":       { "cells": "dynamic_cell_fields", ".": "dynamic_fields" },
				                           "omit_items": { "external_params": "dynamic_params" },
				                           "cast":       { "params": "string[]", "params_values": "string[]" },
																	 "force_set":  [ "allowed_entities", "approvers", "is_run_output_persisted",
																	                 "communication_workspace", "communication_channel" ],
																	 "skip_diff":  [ "allowedUsers", "isRunOutputPersisted", "approvers", "communication" ],
																   "outtype": "json"
			                           },
			"description":             { "type": "string",     "optional": true },
			"timeout_ms":              { "type": "unsigned",   "optional": true, "default": 60000 },
			"allowed_entities":        { "type": "string_set", "optional": true },
			"approvers":               { "type": "string_set", "optional": true },
			"resource_query":          { "type": "string",     "optional": true, "deprecated_for": "allowed_resources_query" },
			"is_run_output_persisted": { "type": "bool",       "optional": true, "step": "is_run_output_persisted", "default": true, "min_ver": "12.3.0" },
			"allowed_resources_query": { "type": "command",    "optional": true, "replaces": "resource_query", "min_ver": "12.3.0" },
			"communication_workspace": { "type": "string",     "optional": true, "min_ver": "12.5.0", "step": "communication_workspace"},
			"communication_channel":   { "type": "string",     "optional": true, "min_ver": "12.5.0", "step": "communication_channel"}
		}
	},

	"resource": {
		"attributes": {
			"type":            { "type": "string",   "computed": true, "value": "RESOURCE" },
			"name":            { "type": "label",    "required": true, "forcenew": true, "skip": true },
			"value":           { "type": "command",  "required": true, "primary": true },
			"description":     { "type": "string",   "optional": true },
			"params":          { "type": "string[]", "optional": true },
			"#units":          { "type": "string",   "optional": true },
			"#resource_type":  { "type": "resource", "optional": true },
			"#user":           { "type": "string",   "optional": true },
			"#read_only":      { "type": "bool",     "optional": true }
		}
	},

	"principal": {
		"attributes": {
			"type":                  { "type": "string",   "computed": true, "value": "PRINCIPAL" },
			"name":                  { "type": "label",    "required": true, "forcenew": true, "skip": true },
			"identity":              { "type": "string",   "required": true, "primary": true },
			"view_limit":            { "type": "int",      "optional": true },
			"action_limit":          { "type": "int",      "optional": true },
			"execute_limit":         { "type": "int",      "optional": true },
			"configure_permission":  { "type": "intbool",  "optional": true },
			"administer_permission": { "type": "intbool",  "optional": true }
		}
	},

	"docs": {
		"objects": {
			"action":    "A command that can be run.\n\nSee the Shoreline [Actions Documentation](https://docs.shoreline.io/actions) for more info.",
			"alarm":     "A condition that triggers Alerts or Actions.\n\nSee the Shoreline [Alarms Documentation](https://docs.shoreline.io/alarms) for more info.",
			"bot":       "An automation that ties an Action to an Alert.\n\nSee the Shoreline [Bots Documentation](https://docs.shoreline.io/bots) for more info.",
			"circuit_breaker": "An automatic rate limit on actions.\n\nSee the Shoreline [CircuitBreakers Documentation](https://docs.shoreline.io/circuit_breakers) for more info.",
			"file":      "A datafile that is automatically copied/distributed to defined Resources.\n\nSee the Shoreline [OpCp Documentation](https://docs.shoreline.io/op/commands/cp) for more info.",
			"integration":  "A third-party integration (e.g. DataDog, NewRelic, etc) .\n\nSee the Shoreline [Metrics Documentation](https://docs.shoreline.io/integrations) for more info.",
			"metric":    "A periodic measurement of a system property.\n\nSee the Shoreline [Metrics Documentation](https://docs.shoreline.io/metrics) for more info.",
			"notebook":  "An interactive notebook of Op commands and user documentation .\n\nSee the Shoreline [Notebook Documentation](https://docs.shoreline.io/ui/notebooks) for more info.",
			"principal": "An authorization group (e.g. Okta groups). Note: Admin privilege (in Shoreline) to create principal objects.",
			"resource":  "A server or compute resource in the system (e.g. host, pod, container).\n\nSee the Shoreline [Resources Documentation](https://docs.shoreline.io/platform/resources) for more info."
		},

		"attributes": {
			"name":                    "The name/symbol for the object within Shoreline and the op language (must be unique, only alphanumeric/underscore).",
			"type":                    "The type of object (i.e., Alarm, Action, Bot, Metric, Resource, or File).",
			"action_limit":            "The number of simultaneous actions allowed for a permissions group.",
			"administer_permission":   "If a permissions group is allowed to perform \"administer\" actions.",
			"allowed_entities":        "The list of users who can run an action or notebook. Any user can run if left empty.",
			"allowed_resources_query": "The list of resources on which an action or notebook can run. No restriction, if left empty.",
			"cells":                   "The data cells inside a notebook.",
			"check_interval":          "Interval (in seconds) between Alarm evaluations.",
			"checksum":                "Cryptographic hash (e.g. md5) of a File Resource.",
			"clear_query":             "The Alarm's resolution condition.",
			"command":                 "A specific action to run.",
			"compile_eligible":        "If the Alarm can be effectively optimized.",
			"complete_long_template":  "The long description of the Action's completion.",
			"complete_short_template": "The short description of the Action's completion.",
			"complete_title_template": "UI title of the Action's completion.",
			"condition_type":          "Kind of check in an Alarm (e.g. above or below) vs a threshold for a Metric.",
			"condition_value":         "Switching value (threshold) for a Metric in an Alarm.",
			"configure_permission":    "If a permissions group is allowed to perform \"configure\" actions.",
			"data":                    "The downloaded (JSON) representation of a Notebook.",
			"description":             "A user-friendly explanation of an object.",
			"destination_path":        "Target location for a copied distributed File object.  See [Op: cp](https://docs.shoreline.io/op/commands/cp).",
			"enabled":                 "If the object is currently enabled or disabled.",
			"error_long_template":     "The long description of the Action's error condition.",
			"error_short_template":    "The short description of the Action's error condition.",
			"error_title_template":    "UI title of the Action's error condition.",
			"event_type":              "Used to tag 'datadog' monitor triggers vs 'shoreline' alarms (default).",
			"execute_limit":           "The number of simultaneous linux (shell) commands allowed for a permissions group.",
			"family":                  "General class for an Action or Bot (e.g., custom, standard, metric, or system check).",
			"file_data":               "Internal representation of a distributed File object's data (computed).",
			"file_deps":               "file object dependencies.",
			"file_length":             "Length, in bytes, of a distributed File object (computed)",
			"fire_long_template":      "The long description of the Alarm's triggering condition.",
			"fire_query":              "The Alarm's trigger condition.",
			"fire_short_template":     "The short description of the Alarm's triggering condition.",
			"fire_title_template":     "UI title of the Alarm's triggering condition.",
			"identity":                "The email address or provider's (e.g. Okta) group-name for a permissions group.",
			"input_file":              "The local source of a distributed File object.",
			"is_run_output_persisted":  "A boolean value denoting whether or not cell outputs should be persisted when running a notebook",
			"md5":                     "The md5 checksum of a file, e.g. filemd5(\"${path.module}/data/example-file.txt\")",
			"metric_name":             "The Alarm's triggering Metric.",
			"monitor_id":              "For 'datadog' monitor triggered bots, the DD monitor identifier.",
			"mute_query":              "The Alarm's mute condition.",
			"params":                  "Named variables to pass to an object (e.g. an Action).",
			"raise_for":               "Where an Alarm is raised (e.g., local to a resource, or global to the system).",
			"res_env_var":             "Result environment variable ... an environment variable used to output values through.",
			"resolve_long_template":   "The long description of the Alarm's resolution.",
			"resolve_short_template":  "The short description of the Alarm's resolution.",
			"resolve_title_template":  "UI title of the Alarm's' resolution.",
			"resource_query":          "A set of Resources (e.g. host, pod, container), optionally filtered on tags or dynamic conditions.",
			"shell":                   "The commandline shell to use (e.g. /bin/sh).",
			"start_long_template":     "The long description when starting the Action.",
			"start_short_template":    "The short description when starting the Action.",
			"start_title_template":    "UI title of the start of the Action.",
			"timeout":                 "Maximum time to wait, in milliseconds.",
			"units":                   "Units of a Metric (e.g., bytes, blocks, packets, percent).",
			"value":                   "The Op statement that defines a Metric or Resource.",
			"view_limit":              "The number of simultaneous metrics allowed for a permissions group.",
			"is_run_output_persisted": "A boolean value denoting whether or not cell outputs should be persisted when running a notebook",
			"communication_workspace": "A string value denoting the slack workspace where notifications related to the object should be sent to.",
			"communication_channel":   "A string value denoting the slack channel where notifications related to the object should be sent to.",
			"service_name":            "The name of a 3rd-party service to integrate with (e.g. 'datadog', or 'newrelic').",
			"api_key":                 "API key for a 3rd-party service integration.",
			"app_key":                 "Application key for a 3rd-party service integration.",
			"permissions_user":        "The user which 3rd-party service integration remediations run as (default 'Shoreline').",
			"dashboard_name":          "The name of a dashboard for 3rd-party service integration (datadog).",
			"webhook_name":            "The name of a webhook for 3rd-party service integration (datadog)."
		}
	}
}
`
