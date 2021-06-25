package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func appendActionLog(msg string) {
	if !DoDebugLog {
		return
	}
	filename := "/tmp/tf-shoreline.log"
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		//panic(err)
		return
	}
	defer f.Close()
	if _, err = f.WriteString(msg); err != nil {
		//panic(err)
		return
	}
}

func runOpCommand(command string, checkResult bool) (string, error) {
	//var GlobalOpts = CliOpts{}
	//if !LoadAuthConfig(&GlobalOpts) {
	//	return "", fmt.Errorf("Failed to load auth credentials")
	//}
	result := ""
	err := error(nil)
	for r := 0; r <= RetryLimit; r += 1 {
		appendActionLog(fmt.Sprintf("Running OpLang command (retries %d/%d)   ---   command:(( %s ))\n", r, RetryLimit, command))
		result, err = ExecuteOpCommand(&GlobalOpts, command)
		if err == nil {
			if !checkResult {
				return result, err
			}
			err = CheckUpdateResult(result)
			if err == nil {
				return result, err
			} else {
				appendActionLog(fmt.Sprintf("Failed OpLang update (retries %d/%d)   ---   error:(( %s ))\n", r, RetryLimit, err.Error()))
			}
		} else {
			appendActionLog(fmt.Sprintf("Failed OpLang command (retries %d/%d)   ---   error:(( %s ))\n", r, RetryLimit, err.Error()))
		}
	}
	return result, err
}

func runOpCommandToJson(command string) (map[string]interface{}, error) {
	result, err := runOpCommand(command, false)
	if err != nil {
		errOut := fmt.Errorf("Failed to execute op '%s': %s", command, err.Error())
		return nil, errOut
	}

	js := map[string]interface{}{}
	// Parsing/Unmarshalling JSON encoding/json
	err = json.Unmarshal([]byte(result), &js)
	if err != nil {
		errOut := fmt.Errorf("Failed to parse json from command '%s': %s", command, err.Error())
		return nil, errOut
	}
	return js, nil
}

func getNamedObjectFromClassDef(name string, typ string, classJs map[string]interface{}) map[string]interface{} {
	baseKey := fmt.Sprintf("get_%s_class.%s_classes", typ, typ)
	baseArray, isArray := GetNestedValueOrDefault(classJs, ToKeyPath(baseKey), []interface{}{}).([]interface{})
	if isArray {
		for _, curJs := range baseArray {
			extrName := GetNestedValueOrDefault(curJs, ToKeyPath("name"), "")
			if name == extrName {
				return curJs.(map[string]interface{})
			}
		}
	}
	return map[string]interface{}{}
}

func CheckUpdateResult(result string) error {
	js := map[string]interface{}{}
	err := json.Unmarshal([]byte(result), &js)
	if err != nil {
		return fmt.Errorf("Failed parse json result from resource update %s", err.Error())
	}

	actions := []string{"define", "delete", "update"}
	types := []string{"resource", "metric", "alarm", "action", "bot", "file"}
	for _, act := range actions {
		for _, typ := range types {
			key := act + "_" + typ
			def := GetNestedValueOrDefault(js, ToKeyPath(key), nil)
			if def != nil {
				errKey := key + ".error.message"
				err := GetNestedValueOrDefault(js, ToKeyPath(errKey), nil)
				if err == nil {
					// success ...
					return nil
				} else {
					errStr := GetInnerErrorStr(CastToString(err))
					// error ...
					return fmt.Errorf("ERROR: %s.\n", errStr)
				}
			}
		}
	}
	return nil
}

// Takes a regex like: "if (?P<if_expr>.*?) then (?P<then_expr>.*?) fi"
// and parses out the named captures (e.g. 'if_expr', 'then_expr')
// into the returned map, with the name as a key, and the match as the value.
func ExtractRegexToMap(expr string, regex string) map[string]interface{} {
	result := map[string]interface{}{}
	re := regexp.MustCompile(regex)
	vals := re.FindStringSubmatch(expr)
	keys := re.SubexpNames()

	// skip index 0, which is the entire expression
	for i := 1; i < len(keys); i++ {
		result[keys[i]] = vals[i]
	}
	return result
}

func ValidateVariableName(name string) bool {
	// match valid variable string names
	matched, _ := regexp.MatchString(`^[_a-zA-Z][_a-zA-Z0-9]*$`, name)
	return matched
}
func ValidateResourceType(name string) bool {
	switch name {
	case "HOST":
		return true
	case "POD":
		return true
	case "CONTAINER":
		return true
	}
	return false
}
func ForceToBool(val interface{}) bool {
	valBool, _ := CastToBoolMaybe(val)
	return valBool
}
func ConvertBoolInt(val interface{}) int {
	valBool, _ := CastToBoolMaybe(val)
	if valBool {
		return 1
	}
	return 0
}

func init() {
	// Set descriptions to support markdown syntax, this will be used in document generation
	// and the language server.
	schema.DescriptionKind = schema.StringMarkdown

	// Customize the content of descriptions when output. For example you can add defaults on
	// to the exported descriptions if present.
	schema.SchemaDescriptionBuilder = func(s *schema.Schema) string {
		desc := s.Description
		if s.Default != nil {
			desc += fmt.Sprintf(" Defaults to `%v`.", s.Default)
		}
		return strings.TrimSpace(desc)
	}
}

func New(version string) func() *schema.Provider {
	return func() *schema.Provider {
		p := &schema.Provider{
			//DataSourcesMap: map[string]*schema.Resource{
			//	"shoreline_datasource": dataSourceShoreline(),
			//},
			ResourcesMap: map[string]*schema.Resource{
				//"shoreline_resource": resourceShorelineBasic(),
				//"shoreline_action":   resourceShorelineAction(),
				"shoreline_action":   resourceShorelineObject(ObjectConfigJsonStr, "action"),
				"shoreline_alarm":    resourceShorelineObject(ObjectConfigJsonStr, "alarm"),
				"shoreline_bot":      resourceShorelineObject(ObjectConfigJsonStr, "bot"),
				"shoreline_metric":   resourceShorelineObject(ObjectConfigJsonStr, "metric"),
				"shoreline_resource": resourceShorelineObject(ObjectConfigJsonStr, "resource"),
				"shoreline_file":     resourceShorelineObject(ObjectConfigJsonStr, "file"),
			},
			Schema: map[string]*schema.Schema{
				"url": {
					Type:     schema.TypeString,
					Required: true,
					ValidateFunc: func(val interface{}, key string) (warns []string, errs []error) {
						if !ValidateApiUrl(val.(string)) {
							errs = append(errs, fmt.Errorf("%q must be of the form %s,\n but got: %s", key, CanonicalUrl, val.(string)))
						}
						return
					},
					DefaultFunc: schema.EnvDefaultFunc("SHORELINE_URL", nil),
				},
				"token": {
					Type:        schema.TypeString,
					Optional:    true,
					Sensitive:   true,
					DefaultFunc: schema.EnvDefaultFunc("SHORELINE_TOKEN", nil),
				},
				"retries": {
					Type:        schema.TypeInt,
					Optional:    true,
					Sensitive:   true,
					DefaultFunc: schema.EnvDefaultFunc("SHORELINE_RETRIES", nil),
				},
				"debug": {
					Type:        schema.TypeBool,
					Optional:    true,
					Sensitive:   true,
					DefaultFunc: schema.EnvDefaultFunc("SHORELINE_DEBUG", nil),
				},
			},
		}

		p.ConfigureContextFunc = configure(version, p)

		return p
	}
}

type apiClient struct {
	// Add whatever fields, client or connection info, etc. here
	// you would need to setup to communicate with the upstream
	// API.
}

func configure(version string, p *schema.Provider) func(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
	return func(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
		AuthUrl = d.Get("url").(string)
		token, hasToken := d.GetOk("token")

		if hasToken {
			SetAuth(&GlobalOpts, AuthUrl, token.(string))
		} else {
			GlobalOpts.Url = AuthUrl
			if !LoadAuthConfig(&GlobalOpts) {
				return nil, diag.Errorf("Failed to load auth credentials file.\n" + GetManualAuthMessage(&GlobalOpts))
			}
			if !selectAuth(&GlobalOpts, AuthUrl) {
				return nil, diag.Errorf("Failed to load auth credentials for %s\n"+GetManualAuthMessage(&GlobalOpts), AuthUrl)
			}
		}

		retries, hasRetry := d.GetOk("retries")
		if hasRetry {
			RetryLimit = retries.(int)
		} else {
			RetryLimit = 0
		}

		debugLog, hasDebugLog := d.GetOk("debug")
		if hasDebugLog {
			DoDebugLog = debugLog.(bool)
		}

		return &apiClient{}, nil
	}
}

////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////

var ObjectConfigJsonStr = `
{
	"action": {
		"attributes": {
			"type":                    { "type": "string",   "computed": true, "value": "ACTION" },
			"name":                    { "type": "label",    "required": true, "forcenew": true },
			"command":                 { "type": "command",  "required": true, "primary": true },
			"description":             { "type": "string",   "optional": true },
			"enabled":                 { "type": "intbool",  "optional": true, "default": false },
			"params":                  { "type": "string[]", "optional": true },
			"res_env_var":             { "type": "string",   "optional": true },
			"resource_query":          { "type": "command",  "optional": true },
			"shell":                   { "type": "string",   "optional": true },
			"timeout":                 { "type": "unsigned", "optional": true, "default": 60 },
			"start_short_template":    { "type": "string",   "optional": true, "step": "start_step_class.short_template" },
			"start_title_template":    { "type": "string",   "optional": true, "step": "start_step_class.title_template" },
			"error_short_template":    { "type": "string",   "optional": true, "step": "error_step_class.short_template" },
			"error_title_template":    { "type": "string",   "optional": true, "step": "error_step_class.title_template" },
			"complete_short_template": { "type": "string",   "optional": true, "step": "complete_step_class.short_template" },
			"complete_title_template": { "type": "string",   "optional": true, "step": "complete_step_class.title_template" },
			"#user":                   { "type": "string",   "optional": true }
		}
	},

	"alarm": {
		"attributes": {
			"type":                   { "type": "string",   "computed": true, "value": "ALARM" },
			"name":                   { "type": "label",    "required": true, "forcenew": true },
			"fire_query":             { "type": "command",  "required": true, "primary": true },
			"clear_query":            { "type": "command",  "optional": true },
			"description":            { "type": "string",   "optional": true },
			"resource_query":         { "type": "command",  "optional": true },
			"enabled":                { "type": "intbool",  "optional": true, "default": false },
			"mute_query":             { "type": "string",   "optional": true },
			"resolve_short_template": { "type": "string",   "optional": true, "step": "clear_step_class.short_template" },
			"resolve_title_template": { "type": "string",   "optional": true, "step": "clear_step_class.title_template", "suppress_null_regex": "^cleared \\w*$" },
			"fire_short_template":    { "type": "string",   "optional": true, "step": "fire_step_class.short_template" },
			"fire_title_template":    { "type": "string",   "optional": true, "step": "fire_step_class.title_template", "suppress_null_regex": "^fired \\w*$" },
			"condition_type":         { "type": "command",  "optional": true, "step": "condition_details.[0].condition_type" },
			"condition_value":        { "type": "float",   "optional": true, "step": "condition_details.[0].condition_value" },
			"metric_name":            { "type": "command",  "optional": true, "step": "condition_details.[0].metric_name" },
			"raise_for":              { "type": "command",  "optional": true, "step": "condition_details.[0].raise_for", "default": "local" },
			"check_interval":         { "type": "command",  "optional": true, "step": "check_interval" },
			"compile_eligible":       { "type": "bool",     "optional": true, "step": "compile_eligible", "default": true },
			"resource_type":          { "type": "command",  "optional": true, "step": "resource_type" },
			"family":                 { "type": "command",  "optional": true, "step": "config_data.family", "default": "custom" }
		}
	},

	"bot": {
		"attributes": {
			"type":             { "type": "string",   "computed": true, "value": "BOT" },
			"name":             { "type": "label",    "required": true, "forcenew": true },
			"command":          { "type": "command",  "required": true, "primary": true, 
				"compound_in": "if (?P<alarm_statement>.*?) then (?P<action_statement>.*?) fi", 
				"compound_out": "if ${alarm_statement} then ${action_statement} fi"
			},
			"description":      { "type": "string",   "optional": true },
			"enabled":          { "type": "intbool",  "optional": true, "default": false },
			"family":           { "type": "command",  "optional": true, "step": "config_data.family", "default": "custom" },
			"action_statement": { "type": "command",  "internal": true },
			"alarm_statement":  { "type": "command",  "internal": true }
		}
	},

	"metric": {
		"attributes": {
			"type":           { "type": "string",   "computed": true, "value": "METRIC" },
			"name":           { "type": "label",    "required": true, "forcenew": true },
			"value":          { "type": "command",   "required": true, "primary": true, "alias_out": "val" },
			"description":    { "type": "string",   "optional": true },
			"units":          { "type": "string",   "optional": true },

			"resource_query": { "type": "command",   "optional": true },
			"shell":          { "type": "string",   "optional": true },
			"timeout":        { "type": "unsigned", "optional": true },
			"params":         { "type": "string[]", "optional": true },
			"res_env_var":    { "type": "string",   "optional": true },
			"#enabled":        { "type": "intbool",  "optional": true, "default": false },
			"#resource_type":  { "type": "resource", "optional": true },
			"#user":           { "type": "string",   "optional": true }
		}
	},

	"resource": {
		"attributes": {
			"type":           { "type": "string",   "computed": true, "value": "RESOURCE" },
			"name":           { "type": "label",    "required": true, "forcenew": true },
			"value":          { "type": "command",  "required": true, "primary": true },
			"description":    { "type": "string",   "optional": true },
			"params":         { "type": "string[]", "optional": true },
			"res_env_var":    { "type": "string",   "optional": true },
			"shell":          { "type": "string",   "optional": true },
			"timeout":        { "type": "unsigned", "optional": true },
			"#units":          { "type": "string",   "optional": true },
			"#resource_type":  { "type": "resource", "optional": true },
			"#user":           { "type": "string",   "optional": true },
			"#read_only":      { "type": "bool",     "optional": true }
		}
	},

	"file": {
		"attributes": {
			"type":             { "type": "string",   "computed": true, "value": "FILE" },
			"name":             { "type": "label",    "required": true, "forcenew": true },
			"destination_path": { "type": "string",   "required": true, "primary": true },
			"description":      { "type": "string",   "optional": true },
			"resource_query":   { "type": "string",   "optional": true },
			"enabled":          { "type": "intbool",  "optional": true, "default": false },
			"input_file":       { "type": "string",   "required": true },
			"file_data":        { "type": "string",   "computed": true },
			"file_length":      { "type": "int",      "computed": true },
			"checksum":         { "type": "string",   "computed": true },
			"#resource_type":    { "type": "resource", "optional": true },
			"#last_modified_timestamp": { "type": "string",   "optional": true }
		}
	},

	"docs": {
		"objects": {
				"action":   "A command that can be run.",
				"alarm":    "A condition that triggers alerts or actions.",
				"bot":      "An automation that ties an action to an alert.",
				"metric":   "A periodic measurement of some property of the system.",
				"resource": "A server or compute resource in the system (e.g. host, pod, container).",
				"file":     "A datafile that is automatically copied/distributed to defined resources."
		},

		"attributes": {
				"type":                    "The type of object (alarm, action, bot, metric, resource, file).",
				"check_interval":          "Interval (in seconds) between alarm evaluations.",
				"checksum":                "Cryptographic hash (e.g. md5) of a file resource.",
				"clear_query":             "A condition that resolves a triggered alarm.",
				"command":                 "A specific action to run.",
				"compile_eligible":        "If the alarm can be effectively optimized.",
				"complete_short_template": "Short description for action completion.",
				"complete_title_template": "UI title for action completion.",
				"condition_type":          "Kind of check in an alarm (e.g. above or below) vs a threshold for a metric.",
				"condition_value":         "Switching value (threshold) for a metric in an alarm.",
				"description":             "A user friendly explanation of an object.",
				"destination_path":        "The location that a distributed file object will be copied to on each resource.",
				"enabled":                 "If the object is currently active or disabled.",
				"error_short_template":    "Short description of an action error condition.",
				"error_title_template":    "UI title for an action error condition.",
				"family":                  "General class for an action or bot (e.g. custom, standard, metric, or system check).",
				"file_data":               "Internal representation of a distributed file object's data (computed).",
				"file_length":             "Length in bytes of a distributed file object (computed)",
				"fire_query":              "A condition that triggers an alarm.",
				"fire_short_template":     "Short description of an alarm trigger condition.",
				"fire_title_template":     "UI title for an alarm trigger condition.",
				"input_file":              "The local source for a distributed file object.",
				"metric_name":             "The metric on which an alarm is triggered.",
				"mute_query":              "A condition that mutes an alarm.",
				"name":                    "The name of the object (must be unique).",
				"params":                  "Named variables to pass to an object (e.g. an action).",
				"raise_for":               "Where an alarm is raised (e.g. local to a resource, or global to the system).",
				"res_env_var":             "Result environment variable ... an environment variable used to output values through.",
				"resolve_short_template":  "Short description of an alarm resolution",
				"resolve_title_template":  "UI title for an alarm resolution.",
				"resource_query":          "A set of resources (e.g. host, pod, container), possibly filtered on tags or dynamic conditions.",
				"shell":                   "The commandline shell to use (e.g. /bin/sh).",
				"start_short_template":    "The short description for start of an action.",
				"start_title_template":    "UI title for the start of an action.",
				"timeout":                 "Maximum time to wait in seconds.",
				"units":                   "Units of a metric (e.g. bytes, blocks, packets, percent).",
				"value":                   "The op statement that defines a metric or resource."
		}
	}
}
`

// old bot
//			"#action_statement": { "type": "command",  "required": true, "primary": true },
//			"#alarm_statement":  { "type": "command",  "required": true }

func resourceShorelineObject(configJsStr string, key string) *schema.Resource {
	params := map[string]*schema.Schema{}

	objects := map[string]interface{}{}
	// Parsing/Unmarshalling JSON encoding/json
	err := json.Unmarshal([]byte(configJsStr), &objects)
	if err != nil {
		WriteMsg("WARNING: Failed to parse JSON config from resourceShorelineObject().\n")
		return nil
	}
	object := GetNestedValueOrDefault(objects, ToKeyPath(key), nil)
	if object == nil {
		WriteMsg("WARNING: Failed to parse JSON config from resourceShorelineObject(%s).\n", key)
		return nil
	}
	attributes := GetNestedValueOrDefault(object, ToKeyPath("attributes"), map[string]interface{}{}).(map[string]interface{})
	primary := "name"
	for k, attrs := range attributes {
		if strings.HasPrefix(k, "#") {
			continue
		}

		// internal objects, i.e. components of compound fields
		internal := GetNestedValueOrDefault(attrs, ToKeyPath("internal"), false).(bool)
		if internal {
			continue
		}

		sch := &schema.Schema{}

		description := CastToString(GetNestedValueOrDefault(objects, ToKeyPath("docs.attributes."+k), ""))
		sch.Description = description

		attrMap := attrs.(map[string]interface{})
		typ := GetNestedValueOrDefault(attrMap, ToKeyPath("type"), "string")
		switch typ {
		case "command":
			sch.Type = schema.TypeString
			sch.DiffSuppressFunc = func(k, old, new string, d *schema.ResourceData) bool {
				// ignore whitespace changes in command strings
				if strings.ReplaceAll(old, " ", "") == strings.ReplaceAll(new, " ", "") {
					return true
				}
				return false
			}
		case "string":
			sch.Type = schema.TypeString
		case "string[]":
			sch.Type = schema.TypeList
			sch.Elem = &schema.Schema{
				Type: schema.TypeString,
			}
		case "bool":
			sch.Type = schema.TypeBool
		case "intbool":
			// special handling to/from backend ("1"/"0")
			sch.Type = schema.TypeBool
		case "float":
			sch.Type = schema.TypeFloat
		case "int":
			sch.Type = schema.TypeInt
		case "unsigned":
			sch.Type = schema.TypeInt
			// non-negative validator
			sch.ValidateFunc = func(val interface{}, key string) (warns []string, errs []error) {
				v := val.(int)
				if v <= 0 {
					errs = append(errs, fmt.Errorf("%q must be > 0, got: %d", key, v))
				}
				return
			}
		case "label":
			sch.Type = schema.TypeString
			// TODO ValidateVariableName()
		case "resource":
			sch.Type = schema.TypeString
			// TODO ValidateResourceType() "^(HOST|POD|CONTAINER)$"
		}
		sch.Optional = GetNestedValueOrDefault(attrMap, ToKeyPath("optional"), false).(bool)
		sch.Required = GetNestedValueOrDefault(attrMap, ToKeyPath("required"), false).(bool)
		sch.Computed = GetNestedValueOrDefault(attrMap, ToKeyPath("computed"), false).(bool)
		sch.ForceNew = GetNestedValueOrDefault(attrMap, ToKeyPath("forcenew"), false).(bool)
		//WriteMsg("WARNING: JSON config from resourceShorelineObject(%s) %s.Optional = %+v.\n", key, k, sch.Optional)
		//WriteMsg("WARNING: JSON config from resourceShorelineObject(%s) %s.Required = %+v.\n", key, k, sch.Required)
		//WriteMsg("WARNING: JSON config from resourceShorelineObject(%s) %s.Computed = %+v.\n", key, k, sch.Computed)
		//defowlt := GetNestedValueOrDefault(attrMap, ToKeyPath("value"), nil)
		defowlt := GetNestedValueOrDefault(attrMap, ToKeyPath("default"), nil)
		if defowlt != nil {
			sch.Default = defowlt
		}
		suppressNullDiffRegex, isStr := GetNestedValueOrDefault(attrMap, ToKeyPath("suppress_null_regex"), nil).(string)
		if isStr {
			sch.DiffSuppressFunc = func(k, old, nu string, d *schema.ResourceData) bool {
				if old == nu {
					return true
				}
				if nu == "" {
					matched, _ := regexp.MatchString(suppressNullDiffRegex, old)
					if matched {
						return true
					}
				}
				return false
			}
		}
		if GetNestedValueOrDefault(attrMap, ToKeyPath("primary"), false).(bool) {
			primary = k
		}
		params[k] = sch
	}

	objDescription := CastToString(GetNestedValueOrDefault(objects, ToKeyPath("docs.objects."+key), ""))

	return &schema.Resource{
		Description: "Shoreline " + key + ". " + objDescription,

		CreateContext: resourceShorelineObjectCreate(key, primary, attributes),
		ReadContext:   resourceShorelineObjectRead(key, attributes),
		UpdateContext: resourceShorelineObjectUpdate(key, attributes),
		DeleteContext: resourceShorelineObjectDelete(key),

		Schema: params,
	}

}

func attrValueString(typ string, key string, val interface{}, attrs map[string]interface{}) string {
	strVal := ""
	attrTyp := GetNestedValueOrDefault(attrs, ToKeyPath(key+".type"), "string").(string)
	switch attrTyp {
	case "command":
		strVal = fmt.Sprintf("%s", val)
	case "string":
		strVal = fmt.Sprintf("\"%s\"", val)
	case "string[]":
		valArr, isArr := val.([]interface{})
		listStr := ""
		sep := ""
		if isArr {
			for _, v := range valArr {
				listStr = listStr + fmt.Sprintf("%s\"%s\"", sep, v)
				sep = ", "
			}
		}
		return "[ " + listStr + " ]"
	case "bool":
		if ForceToBool(val) {
			strVal = fmt.Sprintf("true")
		} else {
			strVal = fmt.Sprintf("false")
		}
	case "intbool": // special handling to/from backend ("1"/"0")
		strVal = fmt.Sprintf("%d", ConvertBoolInt(val))
	case "float":
		strVal = fmt.Sprintf("%f", val)
	case "int":
		strVal = fmt.Sprintf("%d", val)
	case "unsigned":
		strVal = fmt.Sprintf("%d", val)
	case "label":
		strVal = fmt.Sprintf("\"%s\"", val)
	case "resource":
		strVal = fmt.Sprintf("\"%s\"", val)
	}
	return strVal
}

func setFieldViaOp(typ string, attrs map[string]interface{}, name string, key string, val interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	valStr := attrValueString(typ, key, val, attrs)
	appendActionLog(fmt.Sprintf("Setting %s field: '%s'.'%s' :: %+v\n", typ, name, key, val))

	op := fmt.Sprintf("%s.%s = %s", name, key, valStr)

	// TODO Let alias to be a list of fallbacks for versioning,
	//   or have alternate ObjectConfigJsonStr based on backend version,
	//   or let backend return ObjectConfigJsonStr to use.
	alias, isStr := GetNestedValueOrDefault(attrs, ToKeyPath(key+".alias_out"), nil).(string)
	if isStr {
		op = fmt.Sprintf("%s.%s = %s", name, alias, valStr)
	}

	appendActionLog(fmt.Sprintf("Setting with op statement... '%s'\n", op))
	result, err := runOpCommand(op, true)
	if err != nil {
		diags = diag.Errorf("Failed to set %s %s.%s: %s", typ, name, key, err.Error())
		appendActionLog(fmt.Sprintf("Failed to set %s %s.%s: %s\nval: (( %+v ))\nop-statement: %s\n", typ, name, key, val, err.Error(), op))
		return diags
	}
	err = CheckUpdateResult(result)
	if err != nil {
		diags = diag.Errorf("Failed to update %s %s.%s: %s", typ, name, key, err.Error())
		return diags
	}
	return nil
}

func resourceShorelineObjectSetFields(typ string, attrs map[string]interface{}, ctx context.Context, d *schema.ResourceData, meta interface{}, doDiff bool) diag.Diagnostics {
	var diags diag.Diagnostics
	name := d.Get("name").(string)
	// valid-variable-name check (and non-null)
	if typ == "file" {
		infile, exists := d.GetOk("input_file")
		if exists {
			base64Data, ok, fileSize, md5sum := FileToBase64(infile.(string))
			if ok {
				appendActionLog(fmt.Sprintf("file_length is %d (%v)\n", int(fileSize), fileSize))
				d.Set("file_length", int(fileSize))
				d.Set("checksum", md5sum)
				d.Set("file_data", base64Data)
			} else {
				diags = diag.Errorf("Failed to read file object %s", infile)
				return diags
			}
		}
	}

	// TODO handle intbool type (aside from enable)
	writeEnable := false
	enableVal := false
	anyChange := false
	for key, _ := range attrs {

		internal := GetNestedValueOrDefault(attrs, ToKeyPath(key+".internal"), false).(bool)
		if internal {
			continue
		}

		val, exists := d.GetOk(key)
		// NOTE: Terraform reports !exists when a value is explicitly supplied, but matches the 'default'
		if !exists && !d.HasChange(key) {
			appendActionLog(fmt.Sprintf("FieldDoesNotExist: %s: '%s'.'%s' val(%v) HasChange(%v)\n", typ, name, key, val, d.HasChange(key)))
			continue
		}

		// Because OpLang auto-toggles some objects to "disabled" on *any* property change,
		// we have to restore the value as needed.
		if key == "enabled" {
			enableVal, _ = CastToBoolMaybe(val)
			if d.HasChange(key) || !doDiff {
				writeEnable = true
			}
			appendActionLog(fmt.Sprintf("CheckEnableState: %s: '%s' write(%v) val(%v) change(%v) hasChange:(%v) doDiff(%v)\n", typ, name, writeEnable, enableVal, anyChange, d.HasChange(key), doDiff))
			continue
		}
		if doDiff && !d.HasChange(key) {
			continue
		}

		compoundRegex, isStr := GetNestedValueOrDefault(attrs, ToKeyPath(key+".compound_in"), nil).(string)
		if isStr {
			curMap := ExtractRegexToMap(CastToString(val), compoundRegex)
			appendActionLog(fmt.Sprintf("CompoundSet: %s: '%s'.'%s' map(%v) from (( %v ))\n", typ, name, key, curMap, val))

			unchanged := map[string]bool{}
			if doDiff {
				old, _ := d.GetChange(key)
				oldMap := ExtractRegexToMap(CastToString(old), compoundRegex)
				for k, v := range oldMap {
					nu, exists := curMap[k]
					if exists && v == nu {
						unchanged[k] = true
					}
				}
			}

			for k, v := range curMap {
				_, skip := unchanged[k]
				if skip {
					continue
				}
				result := setFieldViaOp(typ, attrs, name, k, v)
				if result != nil {
					return result
				}
			}
			anyChange = true
			continue
		}

		result := setFieldViaOp(typ, attrs, name, key, val)
		if result != nil {
			return result
		}
		anyChange = true
	}

	appendActionLog(fmt.Sprintf("EnableState: %s: '%s' write(%v) val(%v) change(%v)\n", typ, name, writeEnable, enableVal, anyChange))
	// Enabled is automatically toggled to "false" by oplang on any other attribute change.
	// So, it requires special handling.
	if writeEnable || (enableVal && anyChange) {
		act := "enable"
		if !enableVal {
			act = "disable"
		}
		op := fmt.Sprintf("%s %s", act, name)
		appendActionLog(fmt.Sprintf("EnableState: %s: '%s' Op:'%s'\n", typ, name, op))
		result, err := runOpCommand(op, true)
		if err != nil {
			diags = diag.Errorf("Failed to %s (1) %s: %s", act, typ, err.Error())
			return diags
		}
		err = CheckUpdateResult(result)
		if err != nil {
			diags = diag.Errorf("Failed to %s (2) %s: %s", act, typ, err.Error())
			return diags
		}
	}
	return nil
}

func resourceShorelineObjectCreate(typ string, primary string, attrs map[string]interface{}) func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
		// use the meta value to retrieve your client from the provider configure method
		// client := meta.(*apiClient)

		var diags diag.Diagnostics
		name := d.Get("name").(string)
		primaryVal := d.Get(primary)
		idFromAPI := name
		appendActionLog(fmt.Sprintf("Creating %s: '%s' (%v) :: %+v\n", typ, idFromAPI, name, d))

		primaryValStr := attrValueString(typ, primary, primaryVal, attrs)
		//op := fmt.Sprintf("%s %s = \"%s\"", typ, name, primaryVal)
		op := fmt.Sprintf("%s %s = %s", typ, name, primaryValStr)
		//if typ == "bot" {
		//	// special handling for BOT creation statement "bot <name>=
		//	action := d.Get("action_statement").(string)
		//	alarm := d.Get("alarm_statement").(string)
		//	op = fmt.Sprintf("%s %s = if %s then %s fi", typ, name, alarm, action)
		//}
		result, err := runOpCommand(op, true)
		if err != nil {
			// TODO check already exists
			diags = diag.Errorf("Failed to create (1) %s: %s", typ, err.Error())
			return diags
		}
		err = CheckUpdateResult(result)
		if err != nil {
			diags = diag.Errorf("Failed to create (2) %s: %s", typ, err.Error())
			return diags
		}

		diags = resourceShorelineObjectSetFields(typ, attrs, ctx, d, meta, false)
		if diags != nil {
			// delete incomplete object
			resourceShorelineObjectDelete(typ)(ctx, d, meta)
			return diags
		}

		// once the object is ok, set the ID to tell terraform it's valid...
		d.SetId(name)
		// update the data in terraform
		return resourceShorelineObjectRead(typ, attrs)(ctx, d, meta)
	}
}

func resourceShorelineObjectRead(typ string, attrs map[string]interface{}) func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
		// use the meta value to retrieve your client from the provider configure method
		// client := meta.(*apiClient)

		var diags diag.Diagnostics
		name := d.Get("name").(string)
		// valid-variable-name check
		idFromAPI := name
		appendActionLog(fmt.Sprintf("Reading %s: '%s' (%v) :: %+v\n", typ, idFromAPI, name, d))

		op := fmt.Sprintf("list %ss | name = \"%s\"", typ, name)
		js, err := runOpCommandToJson(op)
		if err != nil {
			diags = diag.Errorf("Failed to read %s - %s: %s", typ, name, err.Error())
			return diags
		}

		stepsJs := map[string]interface{}{}

		if typ == "alarm" || typ == "action" || typ == "bot" {
			// extract fields from step objects
			op := fmt.Sprintf("get_%s_class( %s_name = \"%s\" )", typ, typ, name)
			extraJs, err := runOpCommandToJson(op)
			if err != nil {
				diags = diag.Errorf("Failed to read %s - %s: %s", typ, name, err.Error())
				return diags
			}
			stepsJs = getNamedObjectFromClassDef(name, typ, extraJs)
		}

		found := false
		record := map[string]interface{}{}
		symbols, isArray := GetNestedValueOrDefault(js, ToKeyPath("list_type.symbol"), []interface{}{}).([]interface{})
		if isArray {
			for _, s := range symbols {
				sName, isStr := GetNestedValueOrDefault(s, ToKeyPath("attributes.name"), "").(string)
				if isStr && name == sName {
					record = s.(map[string]interface{})
					found = true
				}
			}
		}

		if !found {
			diags = diag.Errorf("Failed to find %s '%s'", typ, name)
			return diags
		}

		for key, attr := range attrs {
			var val interface{}

			internal := GetNestedValueOrDefault(attrs, ToKeyPath(key+".internal"), false).(bool)
			if internal {
				continue
			}

			compoundValue, isStr := GetNestedValueOrDefault(attrs, ToKeyPath(key+".compound_out"), nil).(string)
			if isStr {
				fullVal := compoundValue
				re := regexp.MustCompile(`\$\{\w\w*\}`)
				for expr := re.FindString(fullVal); expr != ""; expr = re.FindString(fullVal) {
					l := len(expr)
					varName := expr[2 : l-1]
					valStr := CastToString(GetNestedValueOrDefault(record, ToKeyPath("attributes."+varName), ""))
					fullVal = strings.Replace(fullVal, expr, valStr, -1)
				}
				val = fullVal
			} else {
				stepPath, isStr := GetNestedValueOrDefault(attr, ToKeyPath("step"), nil).(string)
				if isStr {
					val = GetNestedValueOrDefault(stepsJs, ToKeyPath(stepPath), nil)
				} else {
					val = GetNestedValueOrDefault(record, ToKeyPath("attributes."+key), nil)
				}
				if val == nil {
					continue
				}
			}
			appendActionLog(fmt.Sprintf("Setting %s field: '%s'.'%s' :: %+v\n", typ, name, key, val))
			//typ := GetNestedValueOrDefault(attrs, ToKeyPath(key+".type"), "string").(string)
			//if typ == "string[]" {
			//}
			attrTyp := GetNestedValueOrDefault(attrs, ToKeyPath(key+".type"), "string").(string)
			switch attrTyp {
			case "float":
				d.Set(key, float64(CastToNumber(val)))
			case "int":
				d.Set(key, CastToInt(val))
			case "unsigned":
				d.Set(key, CastToInt(val))
			case "bool":
				d.Set(key, CastToBool(val))
			case "intbool":
				d.Set(key, CastToBool(val))
			case "string[]":
				d.Set(key, CastToArray(val))
			default:
				d.Set(key, val)
			}
		}
		return diags
	}
}

func resourceShorelineObjectUpdate(typ string, attrs map[string]interface{}) func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
		// use the meta value to retrieve your client from the provider configure method
		// client := meta.(*apiClient)

		var diags diag.Diagnostics
		name := d.Get("name").(string)
		appendActionLog(fmt.Sprintf("Updated object '%s': '%s' :: %+v\n", typ, name, d))

		diags = resourceShorelineObjectSetFields(typ, attrs, ctx, d, meta, true)
		if diags != nil {
			// TODO delete incomplete object?
			return diags
		}

		// update the data in terraform
		return resourceShorelineObjectRead(typ, attrs)(ctx, d, meta)
	}
}

func resourceShorelineObjectDelete(typ string) func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
		// use the meta value to retrieve your client from the provider configure method
		// client := meta.(*apiClient)

		var diags diag.Diagnostics
		name := d.Get("name").(string)
		appendActionLog(fmt.Sprintf("deleting %s: '%s' :: %+v\n", typ, name, d))

		op := fmt.Sprintf("delete %s", name)
		result, err := runOpCommand(op, true)
		if err != nil {
			// TODO check already exists
			diags = diag.Errorf("Failed to delete %s: %s", typ, err.Error())
		}
		err = CheckUpdateResult(result)
		if err != nil {
			diags = diag.Errorf("Failed to delete %s: %s", typ, err.Error())
			return diags
		}
		return diags
	}
}
