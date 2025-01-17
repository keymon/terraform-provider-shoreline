// Copyright 2021, Shoreline Software Inc.
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// XXX when we move to go 1.20.X, convert the config to a json file...
// # import _ "embed"
// # go:embed provider_conf.json
// # var ObjectConfigJsonStr

func CanonicalizeUrl(url string) (urlOut string, err error) {
	urlRegexStr := `^(http(s)?://)?(?P<backend_node>([^\\.]*)\.)?(?P<customer>[^\\.]*)\.(?P<region>[^\\.]*)\.ap[ip]\.shoreline-(?P<cluster>[^\\.]*)\.io(/)?$`
	urlBaseStr := "https://${backend_node}${customer}.${region}.api.shoreline-${cluster}.io"
	urlRegex := regexp.MustCompile(urlRegexStr)
	match := urlRegex.FindStringSubmatch(url)
	if len(match) < 4 {
		return "", fmt.Errorf("URL -- %s -- couldn't be mapped to canonical form -- %s -- (%d)\n", url, CanonicalUrl, len(match))
	}
	for i, name := range urlRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			urlBaseStr = strings.Replace(urlBaseStr, "${"+name+"}", match[i], 1)
		}
	}
	return urlBaseStr, nil
}

func StringToJsonArray(data string) ([]interface{}, error) {
	//jsObj := map[string]interface{}{}
	jsObj := []interface{}{}
	jsErr := json.Unmarshal([]byte(data), &jsObj)
	return jsObj, jsErr
}

func Base64ToJsonArray(data string) ([]interface{}, error) {
	jsStr, bError := base64.StdEncoding.DecodeString(data)
	if bError != nil {
		return []interface{}{}, bError
	}
	return StringToJsonArray(string(jsStr))
}

func StringToJson(data string) (map[string]interface{}, error) {
	jsObj := map[string]interface{}{}
	jsErr := json.Unmarshal([]byte(data), &jsObj)
	return jsObj, jsErr
}

func Base64ToJson(data string) (map[string]interface{}, error) {
	// NOTE: there are different encoding styles, i.e.:
	//   b64Str := base64.URLEncoding.EncodeToString([]byte(data))
	//   b64Str := base64.StdEncoding.EncodeToString([]byte(data))
	jsStr, bError := base64.StdEncoding.DecodeString(data)
	if bError != nil {
		return map[string]interface{}{}, bError
	}
	return StringToJson(string(jsStr))
}

func OmitJsonObjectFields(val map[string]interface{}, omitList []interface{}) map[string]interface{} {
	appendActionLog(fmt.Sprintf("Omitting (obj) keys: %+v\n", omitList))
	for _, o := range omitList {
		oStr, isStr := o.(string)
		if isStr {
			delete(val, oStr)
		}
	}
	return val
}

func OmitJsonArrayFields(val *[]interface{}, omitList []interface{}) {
	//appendActionLog(fmt.Sprintf("Omitting (array) keys: %+v\n", omitList))
	for idx, elem := range *val {
		eMap, isMap := elem.(map[string]interface{})
		if isMap {
			(*val)[idx] = OmitJsonObjectFields(eMap, omitList)
		}
	}
}
func JsonFieldsWithValue(val map[string]interface{}, omitList []interface{}) bool {
	for _, omitKeyValue := range omitList {
		keyValueMap, isMap := omitKeyValue.(map[string]interface{})
		if isMap {
			key, ok := keyValueMap["key"].(string)
			if !ok {
				continue
			}
			value, ok := keyValueMap["value"]
			if !ok {
				continue
			}
			if val[key] != value {
				return false
			}
		}
	}
	return true
}
func OmitJsonArrayItems(val *[]interface{}, omitList []interface{}) {
	var idx int = 0
	for _, elem := range *val {
		eMap, isMap := elem.(map[string]interface{})
		if !isMap || !JsonFieldsWithValue(eMap, omitList) {
			(*val)[idx] = elem
			idx++
		}
	}
	*val = (*val)[:idx]
}

func timeSuffixToIntSec(tv string) int {
	sz := len(tv)
	l := sz - 1
	if sz < 1 {
		return 0
	}
	mult := 0
	switch tv[sz-1] {
	case 's':
		mult = 1
	case 'm':
		mult = 60
	case 'h':
		mult = 60 * 60
	case 'd':
		mult = 60 * 60 * 24
	default:
		l = sz
		mult = 1
	}
	val := tv[:l]
	i, err := strconv.Atoi(val)
	if err != nil {
		return -1
	}
	return i * mult
}

func appendActionLogInner(msg string) {
	filename := "/tmp/tf-shoreline.log"
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		//panic(err)
		return
	}
	defer f.Close()
	id := fmt.Sprintf("gid:%d - ", curGoroutineID())
	if _, err = f.WriteString(id + msg); err != nil {
		//panic(err)
		return
	}
}

func appendActionLog(msg string) {
	if !DoDebugLog {
		return
	}
	appendActionLogInner(msg)
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
	types := []string{"resource", "metric", "alarm", "action", "bot", "file", "integration", "notebook"}
	for _, act := range actions {
		for _, typ := range types {
			key := act + "_" + typ
			def := GetNestedValueOrDefault(js, ToKeyPath(key), nil)
			if def != nil {
				errKey := key + ".error.message"
				err := GetNestedValueOrDefault(js, ToKeyPath(errKey), nil)
				if typ == "notebook" && (err == nil || err == "") {
					// have to special-case for notebooks
					err = ""
					errArray := []string{}
					ve, isArray := GetNestedValueOrDefault(js, ToKeyPath(key+".error.validation_errors"), nil).([]interface{})
					if isArray {
						for i, _ := range ve {
							errn, isStr := GetNestedValueOrDefault(js, ToKeyPath(fmt.Sprintf(key+".error.validation_errors.[%d].message", i)), nil).(string)
							if isStr && errn != "" {
								errArray = append(errArray, errn)
							}
							err = strings.Join(errArray, "\n")
						}
					}
				}
				if err == nil || err == "" {
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

	vlen := len(vals)
	// skip index 0, which is the entire expression
	for i := 1; i < len(keys); i++ {
		if i < vlen {
			result[keys[i]] = vals[i]
		}
		// XXX else error, capture group didn't match (but need diags passed in)
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

type VersionRecord struct {
	Valid   bool
	Build   string
	Version string
	Major   int64
	Minor   int64
	Patch   int64
	Error   *error
}

// returns 0,false if either version is invalid
// otherwise, lessThan: -1, equalTo: 0, greaterThan: +1
func CompareVersionRecords(have VersionRecord, want VersionRecord) (gtlteq int, unknown bool) {
	if !have.Valid || !want.Valid {
		return 0, false
	}
	haveVer := []int64{have.Major, have.Minor, have.Patch}
	wantVer := []int64{want.Major, want.Minor, want.Patch}
	for i, want := range wantVer {
		if haveVer[i] < want {
			return -1, true
		}
		if haveVer[i] > want {
			return 1, true
		}
	}
	return 0, true
}

func ExtractVersionData(verStr string) (major int64, minor int64, patch int64, err *error) {
	verRe := regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)
	if verRe.MatchString(verStr) {
		match := verRe.FindStringSubmatch(verStr)
		return CastToInt(match[1]), CastToInt(match[2]), CastToInt(match[3]), nil
	}
	major, minor, patch = 0, 0, 0
	erf := fmt.Errorf("Couldn't find version number in string '%s'", verStr)
	err = &erf
	return
}

func GetBackendVersionInfo() (build string, version string, major int64, minor int64, patch int64, err *error) {
	err = nil
	build = "unknown"
	version = "unknown"
	major, minor, patch = 0, 0, 0
	// op> backend_version
	// ... "get_backend_version": "{ \"tag\": \"release-1.2.3-stuff\", \"build_date\": \"Wed_May_18_00:07:11_UTC_2022\" }", ...
	js, opErr := runOpCommandToJson("backend_version")
	if opErr != nil {
		return
	}
	build = GetNestedValueOrDefault(js, ToKeyPath("get_backend_version"), "unknown").(string)
	buildJs := CastToObject(build)
	if buildJs == nil {
		// TODO set error
		return
	}
	version = GetNestedValueOrDefault(buildJs, ToKeyPath("tag"), "unknown").(string)
	if strings.HasPrefix(version, "stable") || strings.HasPrefix(version, "release") {
		// parse out '\d+\.\d+.\d+' suffix
		major, minor, patch, err = ExtractVersionData(version)
	} else {
		// dev build, special case
		major, minor, patch = 9999, 9999, 9999
	}
	return
}

func GetBackendVersionInfoStruct() VersionRecord {
	var ver VersionRecord
	ver.Build, ver.Version, ver.Major, ver.Minor, ver.Patch, ver.Error = GetBackendVersionInfo()
	ver.Valid = (ver.Error == nil)
	return ver
}

func ParseVersionString(verStr string) VersionRecord {
	var ver VersionRecord
	ver.Version = verStr
	ver.Major, ver.Minor, ver.Patch, ver.Error = ExtractVersionData(verStr)
	ver.Valid = (ver.Error == nil)
	return ver
}

func dataSourceVersionRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	//client := &http.Client{Timeout: 10 * time.Second}

	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics
	build, version, major, minor, patch, err := GetBackendVersionInfo()
	if err != nil {
		diags = diag.Errorf("Failed to read backend_version: %s", (*err).Error())
		return diags
	}

	d.Set("build_info", CastToString(build))
	d.Set("version", CastToString(version))
	d.Set("major", major)
	d.Set("minor", minor)
	d.Set("patch", patch)
	// always run
	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))

	return diags
}

func New(version string) func() *schema.Provider {
	return func() *schema.Provider {
		p := &schema.Provider{
			//DataSourcesMap: map[string]*schema.Resource{
			//	"shoreline_datasource": dataSourceShoreline(),
			//},
			ResourcesMap: map[string]*schema.Resource{
				"shoreline_action":          resourceShorelineObject(ObjectConfigJsonStr, "action"),
				"shoreline_alarm":           resourceShorelineObject(ObjectConfigJsonStr, "alarm"),
				"shoreline_bot":             resourceShorelineObject(ObjectConfigJsonStr, "bot"),
				"shoreline_circuit_breaker": resourceShorelineObject(ObjectConfigJsonStr, "circuit_breaker"),
				"shoreline_file":            resourceShorelineObject(ObjectConfigJsonStr, "file"),
				"shoreline_integration":     resourceShorelineObject(ObjectConfigJsonStr, "integration"),
				"shoreline_metric":          resourceShorelineObject(ObjectConfigJsonStr, "metric"),
				"shoreline_notebook":        resourceShorelineObject(ObjectConfigJsonStr, "notebook"),
				"shoreline_principal":       resourceShorelineObject(ObjectConfigJsonStr, "principal"),
				"shoreline_resource":        resourceShorelineObject(ObjectConfigJsonStr, "resource"),
			},
			DataSourcesMap: map[string]*schema.Resource{
				"shoreline_version": &schema.Resource{
					ReadContext: dataSourceVersionRead,
					Schema: map[string]*schema.Schema{
						"build_info": &schema.Schema{
							Type:     schema.TypeString,
							Computed: true,
						},
						"version": &schema.Schema{
							Type:     schema.TypeString,
							Computed: true,
						},
						"major": &schema.Schema{
							Type:     schema.TypeInt,
							Computed: true,
						},
						"minor": &schema.Schema{
							Type:     schema.TypeInt,
							Computed: true,
						},
						"patch": &schema.Schema{
							Type:     schema.TypeInt,
							Computed: true,
						},
					},
				},
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
					Description: "Customer-specific URL for the Shoreline API server. It should be of the form ```" + CanonicalUrl + "``` .",
				},
				"token": {
					Type:        schema.TypeString,
					Optional:    true,
					Sensitive:   true,
					DefaultFunc: schema.EnvDefaultFunc("SHORELINE_TOKEN", nil),
					Description: "Customer/user-specific authorization token for the Shoreline API server. May be provided via `SHORELINE_TOKEN` env variable.",
				},
				"retries": {
					Type:        schema.TypeInt,
					Optional:    true,
					DefaultFunc: schema.EnvDefaultFunc("SHORELINE_RETRIES", nil),
					Description: "Number of retries for API calls, in case of e.g. transient network failures.",
				},
				"debug": {
					Type:        schema.TypeBool,
					Optional:    true,
					DefaultFunc: schema.EnvDefaultFunc("SHORELINE_DEBUG", nil),
					Description: "Debug logging to `/tmp/tf-shoreline.log`.",
				},
				"min_version": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "Minimum version required on the Shoreline backend (API server).",
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

		var diags diag.Diagnostics = nil

		canonUrl, err := CanonicalizeUrl(AuthUrl)
		if err != nil {
			//return nil, diag.Errorf("Couldn't map URL to canonical form.\n" + err.Error())
			diags = diag.FromErr(err)
			diags[0].Severity = diag.Warning
			canonUrl = AuthUrl
			appendActionLog(fmt.Sprintf("Non-standard url: %s -- to -- %s\n", AuthUrl, canonUrl))
		} else {
			appendActionLog(fmt.Sprintf("Mapped url: %s -- to -- %s\n", AuthUrl, canonUrl))
		}

		if hasToken {
			SetAuth(&GlobalOpts, canonUrl, token.(string))
		} else {
			GlobalOpts.Url = canonUrl
			if !LoadAuthConfig(&GlobalOpts) {
				return nil, diag.Errorf("Failed to load auth credentials file.\n" + GetManualAuthMessage(&GlobalOpts))
			}
			if !selectAuth(&GlobalOpts, canonUrl) {
				return nil, diag.Errorf("Failed to load auth credentials for %s\n"+GetManualAuthMessage(&GlobalOpts), canonUrl)
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

		minVer, hasMinVer := d.GetOk("min_version")
		if hasMinVer {
			var diags diag.Diagnostics
			_, version, major, minor, patch, err := GetBackendVersionInfo()
			if err != nil {
				diags = diag.Errorf("Failed to read backend_version: %s", (*err).Error())
				return nil, diags
			}
			minMajor, minMinor, minPatch, err := ExtractVersionData(minVer.(string))
			if err != nil {
				diags = diag.Errorf("Failed to parse min_version: %s", (*err).Error())
				return nil, diags
			}
			wantVer := []int64{minMajor, minMinor, minPatch}
			haveVer := []int64{major, minor, patch}
			verOk := true
			for i, want := range wantVer {
				if haveVer[i] < want {
					verOk = false
					break
				}
				if haveVer[i] > want {
					break
				}
			}
			if !verOk {
				diags = diag.Errorf("Backend version '%s' (%d, %d, %d) does not meet min_version: '%s' (%d, %d, %d)", version, major, minor, patch, minVer.(string), minMajor, minMinor, minPatch)
				return nil, diags
			}
		}

		return &apiClient{}, diags
	}
}

////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////

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
	for k, _ := range attributes {
		if strings.HasPrefix(k, "#") {
			delete(attributes, k)
		}
	}
	primary := "name"
	for k, attrs := range attributes {
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
			sch.DiffSuppressFunc = func(k, old, nu string, d *schema.ResourceData) bool {
				// ignore whitespace changes in command strings
				if strings.ReplaceAll(old, " ", "") == strings.ReplaceAll(nu, " ", "") {
					return true
				}
				return false
			}
		case "time_s":
			sch.Type = schema.TypeString
			// special case for notebook cells
			sch.DiffSuppressFunc = func(k, old, nu string, d *schema.ResourceData) bool {
				oldT := timeSuffixToIntSec(old)
				nuT := timeSuffixToIntSec(nu)
				appendActionLog(fmt.Sprintf("time_s DiffSuppressFunc: diffing (%s)=(%d) and (%s)=(%d)", old, oldT, nu, nuT))
				if oldT == nuT {
					return true
				}
				return false
			}
		case "b64json":
			sch.Type = schema.TypeString
			// special case for notebook JSON data
			sch.DiffSuppressFunc = func(k, old, nu string, d *schema.ResourceData) bool {
				if old == "" && nu == "" {
					return true
				}
				oldJs, oldErr := StringToJson(old)
				nuJs, nuErr := StringToJson(nu)
				if oldErr != nil || nuErr != nil {
					return false
				}
				// special case top-level notebook "enabled" which may be returned by old backends
				delete(nuJs, "enabled")
				delete(oldJs, "enabled")
				NormalizeNotebookJson(nuJs, attributes)
				NormalizeNotebookJson(oldJs, attributes)
				//appendActionLog(fmt.Sprintf("notebook.data DiffSuppressFunc, new: %+v \n", nuJs))
				//appendActionLog(fmt.Sprintf("notebook.data DiffSuppressFunc, old: %+v \n", oldJs))
				if reflect.DeepEqual(oldJs, nuJs) {
					return true
				}
				return false
			}
			// TODO warn if "data.force_set[i]" fields are present
			//sch.ValidateFunc = func(val interface{}, key string) (warns []string, errs []error) {
			//	v := val.(int)
			//	if v <= 0 {
			//		errs = append(errs, fmt.Errorf("%q must be > 0, got: %d", key, v))
			//	}
			//	return
			//}
		case "string":
			sch.Type = schema.TypeString
		case "string[]":
			sch.Type = schema.TypeList
			sch.Elem = &schema.Schema{
				Type: schema.TypeString,
			}
		case "string_set":
			sch.Type = schema.TypeList
			sch.Elem = &schema.Schema{
				Type: schema.TypeString,
			}
			sch.DiffSuppressFunc = func(k, old, nu string, d *schema.ResourceData) bool {
				//appendActionLog(fmt.Sprintf("string_set DiffSuppressFunc,     k: '%+v'     new: '%+v'     old: '%+v'     d: '%+v' \n", k, old, nu, d))
				// special handling because DiffSuppressFunc doesn't natively work for
				// list-type attributes: https://github.com/hashicorp/terraform-plugin-sdk/issues/477#issue-640263603
				lastDotIndex := strings.LastIndex(k, ".")
				if lastDotIndex != -1 {
					k = string(k[:lastDotIndex])
				}
				oldData, newData := d.GetChange(k)
				if oldData == nil || newData == nil {
					//appendActionLog(fmt.Sprintf("string_set DiffSuppressFunc (one nil),   oldData: '%+v'   newData: '%+v'\n", oldData, newData))
					return false
				}
				oldSortedList := SortListByStrVal(CastToArray(oldData)) // from []any to []string
				newSortedList := SortListByStrVal(CastToArray(newData))
				//appendActionLog(fmt.Sprintf("string_set DiffSuppressFunc (sorted lists),   oldList: '%+v'   newList: '%+v'\n", oldSortedList, newSortedList))
				return reflect.DeepEqual(oldSortedList, newSortedList)
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
			// ValidateVariableName()
			sch.ValidateFunc = func(val interface{}, key string) (warns []string, errs []error) {
				re := regexp.MustCompile("^[a-zA-Z0-9_]*$")
				v, isStr := val.(string)
				if !isStr || (!re.MatchString(v)) {
					errs = append(errs, fmt.Errorf("%q must be an alphanumeric/underscore string, got: '%+v'", key, val))
				} else {
					res := regexp.MustCompile("^[a-zA-Z_]")
					if !res.MatchString(v) {
						errs = append(errs, fmt.Errorf("%q must start with a letter or underscore, got: '%+v'", key, val))
					}
				}
				return
			}
		case "resource":
			sch.Type = schema.TypeString
			// TODO ValidateResourceType() "^(HOST|POD|CONTAINER)$"
		}
		sch.Optional = GetNestedValueOrDefault(attrMap, ToKeyPath("optional"), false).(bool)
		sch.Required = GetNestedValueOrDefault(attrMap, ToKeyPath("required"), false).(bool)
		sch.Computed = GetNestedValueOrDefault(attrMap, ToKeyPath("computed"), false).(bool)
		sch.ForceNew = GetNestedValueOrDefault(attrMap, ToKeyPath("forcenew"), false).(bool)
		deprecated := GetNestedValueOrDefault(attrMap, ToKeyPath("deprecated"), false).(bool)
		deprField := GetNestedValueOrDefault(attrMap, ToKeyPath("deprecated_for"), "").(string)
		if deprecated {
			sch.Deprecated = fmt.Sprintf("Field '%s' is obsolete.", k)
			sch.Description = "**Deprecated** " + sch.Deprecated + " " + description
		}
		if deprField != "" {
			sch.Deprecated = fmt.Sprintf("Please use '%s' instead.", deprField)
			sch.ConflictsWith = []string{deprField}
			sch.Description = "**Deprecated** " + sch.Deprecated + " " + description
			// XXX does this need diff suppression?
			//sch.DiffSuppressFunc = func(k, old, nu string, d *schema.ResourceData) bool { }
		}
		replacesField := GetNestedValueOrDefault(attrMap, ToKeyPath("replaces"), "").(string)
		if replacesField != "" {
			sch.ConflictsWith = []string{replacesField}
		}
		//WriteMsg("WARNING: JSON config from resourceShorelineObject(%s) %s.Optional = %+v.\n", key, k, sch.Optional)
		//WriteMsg("WARNING: JSON config from resourceShorelineObject(%s) %s.Required = %+v.\n", key, k, sch.Required)
		//WriteMsg("WARNING: JSON config from resourceShorelineObject(%s) %s.Computed = %+v.\n", key, k, sch.Computed)
		//defowlt := GetNestedValueOrDefault(attrMap, ToKeyPath("value"), nil)
		defowlt := GetNestedValueOrDefault(attrMap, ToKeyPath("default"), nil)
		if defowlt != nil {
			//appendActionLogInner(fmt.Sprintf("NOTE: DEFAULT resourceShorelineObject(%s) %s.Default = %+v.\n", key, k, defowlt))
			sch.Default = defowlt
		}
		suppressNullDiffRegex, isStr := GetNestedValueOrDefault(attrMap, ToKeyPath("suppress_null_regex"), nil).(string)
		if isStr {
			sch.DiffSuppressFunc = func(k, old, nu string, d *schema.ResourceData) bool {
				appendActionLog(fmt.Sprintf("suppressNullDiff check: '%s': '%s' -- vs -- '%s'\n", suppressNullDiffRegex, old, nu))
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
		matchNull := GetNestedValueOrDefault(attrMap, ToKeyPath("match_null"), nil)
		if matchNull != nil {
			sch.DiffSuppressFunc = func(k, old, nu string, d *schema.ResourceData) bool {
				//appendActionLog(fmt.Sprintf("suppressNullMatch check: '%v': '%s' -- vs -- '%s'\n", matchNull, old, nu))
				if old == nu {
					return true
				}
				if old == "" && nu == matchNull {
					return true
				}
				if nu == "" && old == matchNull {
					return true
				}
				return false
			}
		}

		// NOTE: This actually messes up the file objects. Need a suppress function that's just for acceptance test comparisions.
		//notStored, isBool := GetNestedValueOrDefault(attrMap, ToKeyPath("not_stored"), nil).(bool)
		//if isBool && notStored {
		//	sch.DiffSuppressFunc = func(k, old, nu string, d *schema.ResourceData) bool {
		//		//appendActionLog(fmt.Sprintf("Not Stored Value: '%s': '%s' -- vs -- '%s'\n", k, old, nu))
		//		if old == nu || nu == "" || old == "" {
		//			return true
		//		}
		//		return false
		//	}
		//}

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
		Importer:      &schema.ResourceImporter{State: schema.ImportStatePassthrough},

		Schema: params,
	}

}

func NormalizeNotebookJsonArray(arr []interface{}) {
	for _, v := range arr {
		theMap, isMap := v.(map[string]interface{})
		if isMap {
			NormalizeNotebookJson(theMap, nil)
		}
	}
}

func NormalizeNotebookJson(object map[string]interface{}, attributes map[string]interface{}) {
	toRemove := map[string]bool{}
	if attributes != nil {
		skip_diff, ok := GetNestedValueOrDefault(attributes, ToKeyPath("data.skip_diff"), []interface{}{}).([]interface{})
		if ok {
			for _, k := range skip_diff {
				toRemove[CastToString(k)] = true
			}
		}
	}
	for k, v := range object {
		arr, isArray := v.([]interface{})
		if toRemove[CastToString(k)] {
			appendActionLog(fmt.Sprintf("NormalizeNotebookJson() toRemove: '%+v'\n", k))
			delete(object, k)
		} else if isArray {
			// remove empty lists (e.g. external_params)
			if len(arr) == 0 {
				delete(object, k)
			} else {
				// NOTE: In future, may need to sort nested non-ordinal lists (ala top-level allowed_entities).
				NormalizeNotebookJsonArray(arr)
			}
		} else {
			theMap, isMap := v.(map[string]interface{})
			if isMap {
				NormalizeNotebookJson(theMap, nil)
			} else {
				if k == "external_params" && v == nil {
					delete(object, k)
				}
			}
		}
	}
}

func EscapeString(val interface{}) string {
	out := fmt.Sprintf("%s", val)

	slash := regexp.MustCompile(`\\`)
	out = slash.ReplaceAllString(out, "\\\\")
	quote := regexp.MustCompile(`"`)
	out = quote.ReplaceAllString(out, "\\\"")

	return out
}

func SortListByStrVal(val []interface{}) []interface{} {
	sortedCopy := make([]interface{}, len(val), len(val))
	copy(sortedCopy, val)
	sort.Slice(sortedCopy, func(i, j int) bool {
		s1, s2 := "\"\"", "\"\""
		if sortedCopy[i] != nil {
			s1 = fmt.Sprintf("\"%s\"", EscapeString(sortedCopy[i]))
		}
		if sortedCopy[j] != nil {
			s2 = fmt.Sprintf("\"%s\"", EscapeString(sortedCopy[j]))
		}
		return s1 < s2
	})
	return sortedCopy
}

func attrValueDefault(attrTyp string) interface{} {
	switch attrTyp {
	case "command":
		return ""
	case "time_s":
		return ""
	case "b64json":
		return ""
	case "string":
		return ""
	case "string[]":
		return []string{}
	case "string_set":
		return []string{}
	case "bool":
		return false
	case "intbool": // special handling to/from backend ("1"/"0")
		return false
	case "float":
		return float64(0)
	case "int":
		return int(0)
	case "unsigned":
		return uint(0)
	case "label":
		return ""
	case "resource":
		return ""
	}
	return ""
}

func attrValueString(typ string, key string, val interface{}, attrs map[string]interface{}) string {
	strVal := ""
	attrTyp := GetNestedValueOrDefault(attrs, ToKeyPath(key+".type"), "string").(string)
	optional := GetNestedValueOrDefault(attrs, ToKeyPath(key+".optional"), false).(bool)
	switch attrTyp {
	case "command":
		if optional && val == "" {
			strVal = "\"\""
		} else {
			strVal = fmt.Sprintf("%s", val)
		}
	case "time_s":
		strVal = fmt.Sprintf("%s", val)
	case "b64json":
		jsStr, isStr := val.(string)
		if !isStr {
			jsStr = ""
		}
		strVal = fmt.Sprintf("\"%s\"", base64.StdEncoding.EncodeToString([]byte(jsStr)))
	case "string":
		strVal = fmt.Sprintf("\"%s\"", EscapeString(val))
	case "string[]":
		valArr, isArr := val.([]interface{})
		listStr := ""
		sep := ""
		if isArr {
			for _, v := range valArr {
				if v == nil {
					listStr = listStr + fmt.Sprintf("%s\"\"", sep)
				} else {
					listStr = listStr + fmt.Sprintf("%s\"%s\"", sep, EscapeString(v))
				}
				sep = ", "
			}
		}
		return "[ " + listStr + " ]"
	case "string_set":
		valArr, isArr := val.([]interface{})
		listStr := ""
		sep := ""
		if isArr {
			for _, v := range valArr {
				if v == nil {
					listStr = listStr + fmt.Sprintf("%s\"\"", sep)
				} else {
					listStr = listStr + fmt.Sprintf("%s\"%s\"", sep, EscapeString(v))
				}
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
		//appendActionLog(fmt.Sprintf("Setting %s aliased field: '%s'->'%s'.'%s' :: %+v\n", typ, name, alias, key, val))
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

func getRemoteFileAttr(name string, key string) string {
	pathAttrCmd := fmt.Sprintf("%s.%s", name, key)
	pathJson, err := runOpCommandToJson(pathAttrCmd)
	if err != nil {
		return ""
	}
	uri, isStr := GetNestedValueOrDefault(pathJson, ToKeyPath("get_file_attribute"), nil).(string)
	// "get file attribute failed: field does not exist"
	if !isStr || strings.Contains(uri, "failed:") || strings.Contains(uri, "field does not exist") {
		return ""
	}
	return uri
}

func setFieldInner(key string, val interface{}, name string, typ string, attrs map[string]interface{}, ctx context.Context, d *schema.ResourceData, meta interface{}, doDiff bool, isCreate bool, forcedChangeKeys map[string]bool, forcedChangeVals map[string]interface{}) (bool, diag.Diagnostics) {
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
				return false, result
			}
		}
		return true, nil
	}

	result := diag.Diagnostics(nil)
	if forcedChangeKeys[key] {
		result = setFieldViaOp(typ, attrs, name, key, forcedChangeVals[key])
	} else {
		result = setFieldViaOp(typ, attrs, name, key, val)

		// on failure, if field is deprecated and renamed, try the new name
		deprecatedFor := GetNestedValueOrDefault(attrs, ToKeyPath(key+".deprecated_for"), "").(string)
		if deprecatedFor != "" && result != nil {
			appendActionLog(fmt.Sprintf("Set deprecated/renamed field : %s: '%s'.'%s'->'%s'  val:'%v'\n", typ, name, key, deprecatedFor, val))
			result = setFieldViaOp(typ, attrs, name, deprecatedFor, val)
		}
	}
	if result != nil {
		return false, result
	}
	return true, nil
}

func shouldSkipSetField(key string, val interface{}, name string, typ string, attrs map[string]interface{}, ctx context.Context, d *schema.ResourceData, meta interface{}, doDiff bool, isCreate bool, forcedChangeKeys map[string]bool, forcedChangeVals map[string]interface{}, backendVersion VersionRecord) (bool, diag.Diagnostics) {
	skip := GetNestedValueOrDefault(attrs, ToKeyPath(key+".skip"), false).(bool)
	if skip {
		appendActionLog(fmt.Sprintf("Set (skipping explicit): %s: '%s'.'%s'\n", typ, name, key))
		return true, nil
	}

	internal := GetNestedValueOrDefault(attrs, ToKeyPath(key+".internal"), false).(bool)
	if internal {
		appendActionLog(fmt.Sprintf("Set (skipping internal): %s: '%s'.'%s'\n", typ, name, key))
		return true, nil
	}
	proxy := GetNestedValueOrDefault(attrs, ToKeyPath(key+".proxy"), "").(string)
	if proxy != "" {
		appendActionLog(fmt.Sprintf("Set (skipping proxy): %s: '%s'.'%s'\n", typ, name, key))
		return true, nil
	}

	min_ver := GetNestedValueOrDefault(attrs, ToKeyPath(key+".min_ver"), "").(string)
	if min_ver != "" {
		minVer := ParseVersionString(min_ver)
		// XXX check minVer.Error and complain about version string
		gtlteq, valid := CompareVersionRecords(backendVersion, minVer)
		if valid && gtlteq < 0 {
			// NOTE: see below for errata on GetOk().exists
			val, exists := d.GetOk(key)
			defowlt := GetNestedValueOrDefault(attrs, ToKeyPath(key+".default"), nil)
			if defowlt == nil {
				attrTyp := GetNestedValueOrDefault(attrs, ToKeyPath(key+".type"), "string").(string)
				defowlt = attrValueDefault(attrTyp)
			}
			appendActionLog(fmt.Sprintf("Set (checking min_ver): %s: '%s'.'%s' exists(%v) val(%v) default(%v) ver(%v) backend_ver(%v)\n", typ, name, key, exists, val, defowlt, min_ver, backendVersion.Version))
			// NOTE: because of the bug in GetOk(), we can't know for sure if the value is set in the TF HCL
			//   e.g. value=<unset>, default=true -> exists==true
			//        value=false,   default=true -> exists==false
			// So, be conservative, and only complain if it's different than the default:
			if val != nil && val != defowlt {
				// XXX error or warning? (Hashi plugin SDK v2 doesn't seem to support warnings)
				//diags.AddWarning("Below minimum version.", fmt.Sprintf("Field %s.%s requires minimum version %s, skipping...", name, key, min_ver))
				diags := diag.Errorf("Field '%s.%s' requires minimum version '%s', but backend is '%s'", name, key, min_ver, backendVersion.Version)
				return false, diags
			}
			appendActionLog(fmt.Sprintf("Set (skipping): %s: '%s'.'%s' exists(%v) val(%v) default(%v) ver(%v) backend_ver(%v)\n", typ, name, key, exists, val, defowlt, min_ver, backendVersion.Version))
			return true, nil
		}
	}
	return false, nil
}

func resourceShorelineObjectSetFields(typ string, attrs map[string]interface{}, ctx context.Context, d *schema.ResourceData, meta interface{}, doDiff bool, isCreate bool) diag.Diagnostics {
	var diags diag.Diagnostics
	name := d.Get("name").(string)
	// valid-variable-name check (and non-null)
	//appendActionLog(fmt.Sprintf("RESOURCE TYPE IS: %s\n", typ))

	needVersion := false
	writeEnable := false
	enableVal := false
	anyChange := false
	// fields that have to be explicitly set (e.g. notebook fields both in JSON and explicit TF)
	forcedUpdate := map[string]bool{}
	// computed file properties
	forcedChangeKeys := map[string]bool{}
	forcedChangeVals := map[string]interface{}{}

	for key, _ := range attrs {
		proxy := GetNestedValueOrDefault(attrs, ToKeyPath(key+".proxy"), "").(string)
		if proxy != "" {
			proxyKeys := strings.Split(proxy, ",")
			for _, k := range proxyKeys {
				forcedChangeKeys[k] = true
			}
		}
		min_ver := GetNestedValueOrDefault(attrs, ToKeyPath(key+".min_ver"), "").(string)
		if min_ver != "" {
			needVersion = true
		}
	}

	var backendVersion VersionRecord
	backendVersion.Valid = false
	if needVersion {
		backendVersion = GetBackendVersionInfoStruct()
	}

	if typ == "file" {
		infile, exists := d.GetOk("input_file")
		if exists {
			uri := getRemoteFileAttr(name, "uri")
			fileIsRemote := true
			if uri == "" {
				fileIsRemote = false
			}
			base64Data, ok, fileSize, md5sum := FileToBase64(infile.(string), fileIsRemote)
			if fileIsRemote {
				base64Data = fmt.Sprintf(":%s", uri)
			}
			if ok {
				appendActionLog(fmt.Sprintf("file_length is %d (%v)\n", int(fileSize), fileSize))
				if forcedChangeKeys["file_data"] {
					forcedChangeVals["file_length"] = int(fileSize)
					forcedChangeVals["checksum"] = md5sum
					forcedChangeVals["file_data"] = base64Data
				}
				d.Set("file_length", int(fileSize))
				d.Set("checksum", md5sum)
				d.Set("file_data", base64Data)
				if fileIsRemote {
					presignedUrl := getRemoteFileAttr(name, "presigned_put")
					if presignedUrl == "" {
						diags = diag.Errorf("Failed to get presigned url for file object %s", name)
						return diags
					}
					err := UploadFileHttps(infile.(string), presignedUrl, "")
					if err != nil {
						diags = diag.Errorf("Failed to upload to presigned url for file object %s -- %s", name, err.Error())
						return diags
					}
				}
			} else {
				diags = diag.Errorf("Failed to read file object %s", infile)
				return diags
			}
		}
	}

	// Have to explicitly set "data" first, as it overrides some other attributes (e.g. "approvers")
	if typ == "notebook" {
		key := "data"
		val, exists := d.GetOk(key)
		// NOTE: Terraform reports !exists when a value is explicitly supplied, but matches the 'default'
		if exists || d.HasChange(key) {
			changed, diags := setFieldInner(key, val, name, typ, attrs, ctx, d, meta, doDiff, isCreate, forcedChangeKeys, forcedChangeVals)
			if diags != nil {
				return diags
			}
			if changed {
				anyChange = true
			}
		}
		forced, hasForced := GetNestedValueOrDefault(attrs, ToKeyPath(key+".force_set"), false).([]interface{})
		if hasForced {
			for _, k := range forced {
				forcedUpdate[CastToString(k)] = true
			}
		}
	}

	skipKeys := map[string]bool{}
	if typ == "notebook" {
		skipKeys["data"] = true
		skipKeys["approvers"] = true
		skipKeys["allowed_entities"] = true
	}
	orderedAttrs := []string{}
	for key, _ := range attrs {
		if skipKeys[key] != true {
			orderedAttrs = append(orderedAttrs, key)
		} else {
			appendActionLog(fmt.Sprintf("Notebook skipping key: %s\n", key))
		}
	}
	if typ == "notebook" {
		// XXX Hack: work around backend issue with wacky data-dependent ordering
		aVal, _ := d.Get("allowed_entities").([]interface{})
		//appendActionLog(fmt.Sprintf("Notebook allowed_entities has len: %v\n", len(aVal)))
		if len(aVal) > 0 {
			orderedAttrs = append(orderedAttrs, "allowed_entities")
			orderedAttrs = append(orderedAttrs, "approvers")
		} else {
			orderedAttrs = append(orderedAttrs, "approvers")
			orderedAttrs = append(orderedAttrs, "allowed_entities")
		}
	}

	for _, key := range orderedAttrs {
		// NOTE: GetOk() has bugs: it checks vs 0/false/"" instead of presence of an explicit value, or even equality to the default
		val, exists := d.GetOk(key)

		skip, diags := shouldSkipSetField(key, val, name, typ, attrs, ctx, d, meta, doDiff, isCreate, forcedChangeKeys, forcedChangeVals, backendVersion)
		if diags != nil {
			return diags
		}
		if skip {
			continue
		}
		if typ == "notebook" && key == "data" {
			// set first above, skip here
			continue
		}

		forceSet := false
		// CS-336 workaround: Force explicit set of action_statement/alarm_statement to patch quoting issue
		_, botEnvDefined := os.LookupEnv("BOT_SKIP_PATCH")
		isPrimary := GetNestedValueOrDefault(attrs, ToKeyPath(key+".primary"), false).(bool)
		if isCreate && isPrimary && typ == "bot" {
			if botEnvDefined {
				appendActionLog(fmt.Sprintf("Bot skipping post-ctor set: %s: '%s'.'%s' HasChange(%v)\n", typ, name, key, d.HasChange(key)))
				// primary value is set on creation, and redundant set currently triggers an issue with bots
				continue
			} else {
				appendActionLog(fmt.Sprintf("Bot running post-ctor set: %s: '%s'.'%s'  HasChange(%v)\n", typ, name, key, d.HasChange(key)))
				forceSet = true
			}
		}

		// NOTE: Terraform reports !exists when a value is explicitly supplied, but matches the 'default'
		if !exists && !d.HasChange(key) && !forceSet && !forcedChangeKeys[key] && !forcedUpdate[key] {
			defowlt := GetNestedValueOrDefault(attrs, ToKeyPath(key+".default"), nil)
			appendActionLog(fmt.Sprintf("FieldDoesNotExist: %s: '%s'.'%s' val(%v) HasChange(%v), forceSet(%v) isCreate(%v) default(%v)\n", typ, name, key, val, d.HasChange(key), forceSet, isCreate, defowlt))
			// Handle GetOk() bug...
			if isCreate {
				if defowlt == nil || val == defowlt {
					continue
				}
			} else {
				continue
			}
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
		if doDiff && !d.HasChange(key) && !forcedChangeKeys[key] && !forcedUpdate[key] {
			continue
		}

		changed, diags := setFieldInner(key, val, name, typ, attrs, ctx, d, meta, doDiff, isCreate, forcedChangeKeys, forcedChangeVals)
		if diags != nil {
			return diags
		}
		if changed {
			anyChange = true
		}
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
		//appendActionLog(fmt.Sprintf("primaryValStr is ((( %+v )))\n", primaryValStr))
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
			// TODO check if already exists
			diags = diag.Errorf("Failed to create (1) %s: %s", typ, err.Error())
			return diags
		}
		err = CheckUpdateResult(result)
		if err != nil {
			diags = diag.Errorf("Failed to create (2) %s: %s", typ, err.Error())
			return diags
		}

		diags = resourceShorelineObjectSetFields(typ, attrs, ctx, d, meta, false, true)
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

// returns skip, value, diagnostics
func resourceShorelineObjectReadSingleAttr(name string, typ string, key string, attrs map[string]interface{}, record map[string]interface{}, stepsJs map[string]interface{}, d *schema.ResourceData) (bool, interface{}, diag.Diagnostics) {
	var val interface{}
	attr := GetNestedValueOrDefault(attrs, ToKeyPath(key), map[string]interface{}{})

	if strings.HasPrefix(key, "#") {
		// skip commented fields
		return true, nil, nil
	}

	internal := GetNestedValueOrDefault(attrs, ToKeyPath(key+".internal"), false).(bool)
	if internal {
		// skip internal fields
		return true, nil, nil
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
			if stepPath == "." {
				val = stepsJs
			} else {
				val = GetNestedValueOrDefault(stepsJs, ToKeyPath(stepPath), nil)
			}

			// special handling (notebooks)... field is base64 outgoing, and json incoming
			attrTyp := GetNestedValueOrDefault(attrs, ToKeyPath(key+".type"), "string").(string)
			if attrTyp == "b64json" {
				// The code below that omits fields/objects will modify 'val', so we make a copy
				val = DeepCopy(val)
				// handle cast-map, as get_notebook_class() returns objects with some string-wrapped sub-fields
				castMap := GetNestedValueOrDefault(attr, ToKeyPath("cast"), map[string]interface{}{}).(map[string]interface{})
				for castPath, castType := range castMap {
					cur := GetNestedValueOrDefault(val, ToKeyPath(castPath), nil)
					if cur != nil {
						// TODO add additional types as needed
						switch castType {
						case "string[]":
							SetNestedValue(val, ToKeyPath(castPath), CastToArray(cur))
						case "string_set":
							SetNestedValue(val, ToKeyPath(castPath), CastToArray(cur))
						case "object":
							SetNestedValue(val, ToKeyPath(castPath), CastToObject(cur))
						}
					}
				}

				// handle omit map, and nested deletions, as get_notebook_class() returns objects with dynamic/temporary fields
				omitMap := GetNestedValueOrDefault(attr, ToKeyPath("omit"), map[string]interface{}{}).(map[string]interface{})
				// "." has to be last, or it will wipe out other objects
				omitPaths := []string{}
				hasDot := false
				for omitPath, _ := range omitMap {
					if omitPath != "." {
						omitPaths = append(omitPaths, omitPath)
					} else {
						hasDot = true
					}
				}
				if hasDot {
					omitPaths = append(omitPaths, ".")
				}
				for _, omitPath := range omitPaths {
					omitTag := omitMap[omitPath]
					appendActionLog(fmt.Sprintf("Omit path:'%+v' tag: '%+v'\n", omitPath, omitTag))
					var cur interface{}
					if omitPath == "." {
						cur = val
					} else {
						cur = GetNestedValueOrDefault(val, ToKeyPath(omitPath), nil)
					}
					omitTagStr, isStr := omitTag.(string)
					if !isStr {
						// skip omit fields
						return true, nil, nil
					}
					omitList, isList := GetNestedValueOrDefault(stepsJs, ToKeyPath(omitTagStr), []interface{}{}).([]interface{})
					//appendActionLog(fmt.Sprintf("Omit-list path:'%+v' tag: '%+v' list:'%+v'\n", omitPath, omitTag, omitList))
					if cur != nil && isList {
						if typ == "notebook" && omitPath == "." {
							// NOTE: The top-level object returned by get_notebook_class contains most/all of the object attributes.
							// So remove them from the inner object
							for akey, _ := range attrs {
								omitList = append(omitList, akey)
							}
							omitList = append(omitList, "enabled")
						}
						switch cur.(type) {
						case map[string]interface{}:
							OmitJsonObjectFields(cur.(map[string]interface{}), omitList)
						case []interface{}:
							curArr := cur.([]interface{})
							OmitJsonArrayFields(&curArr, omitList)
						}
						if omitPath == "." {
							val = cur
						} else {
							SetNestedValue(val, ToKeyPath(omitPath), cur)
						}
					}
				}

				// handle dynamic parameters (eg. datadog external params)
				omitMap = GetNestedValueOrDefault(attr, ToKeyPath("omit_items"), map[string]interface{}{}).(map[string]interface{})
				for omitPath, omitTag := range omitMap {
					cur, isList := GetNestedValueOrDefault(val, ToKeyPath(omitPath), nil).([]interface{})
					if cur == nil || !isList {
						// skip ...
						return true, nil, nil
					}
					omitTagStr, isStr := omitTag.(string)
					if !isStr {
						// skip omit fields
						return true, nil, nil
					}
					omitList, isList := GetNestedValueOrDefault(stepsJs, ToKeyPath(omitTagStr), []interface{}{}).([]interface{})
					if !isList {
						// skip omit fields
						return true, nil, nil
					}
					OmitJsonArrayItems(&cur, omitList)
					SetNestedValue(val, ToKeyPath(omitPath), cur)
				}

				b, err := json.Marshal(val)
				if err != nil {
					diags := diag.Errorf("Failed to marshall JSON %s:%s '%s'", typ, key, name)
					return false, nil, diags
				}
				//val = base64.URLEncoding.EncodeToString(b)
				val = string(b)
			}
		} else {
			val = GetNestedValueOrDefault(record, ToKeyPath("attributes."+key), nil)
		}
	}
	return false, val, nil
}

func resourceShorelineObjectRead(typ string, attrs map[string]interface{}) func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
		// use the meta value to retrieve your client from the provider configure method
		// client := meta.(*apiClient)

		var diags diag.Diagnostics
		name := d.Get("name").(string)
		if name == "" {
			// fallback for 'terraform import'
			name = d.Id()
		}
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

		if typ == "alarm" || typ == "action" || typ == "bot" || typ == "integration" || typ == "notebook" {
			// extract fields from step objects
			op := fmt.Sprintf("get_%s_class( %s_name = \"%s\" )", typ, typ, name)
			extraJs, err := runOpCommandToJson(op)
			if err != nil {
				diags = diag.Errorf("Failed to read %s - %s: %s", typ, name, err.Error())
				return diags
			}
			stepsJs = getNamedObjectFromClassDef(name, typ, extraJs)

			if typ == "integration" {
				// unpack attributes.configuration (integration) which is a string-encoded JSON value
				confStr, hasConfStr := GetNestedValueOrDefault(stepsJs, ToKeyPath("params"), nil).(string)
				if hasConfStr {
					conf := map[string]interface{}{}
					err := json.Unmarshal([]byte(confStr), &conf)
					if err == nil {
						SetNestedValue(stepsJs, ToKeyPath("params_unpack"), conf)
					}
				}
			}
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

		for key, _ := range attrs {
			skip, val, diags := resourceShorelineObjectReadSingleAttr(name, typ, key, attrs, record, stepsJs, d)
			if diags != nil {
				return diags
			}
			if skip {
				appendActionLog(fmt.Sprintf("Reading (skip) %s field: '%s'.'%s' :: %+v\n", typ, name, key, val))
				continue
			}

			// if there's an obsolete prior name, with a value set, skip
			replaces := GetNestedValueOrDefault(attrs, ToKeyPath(key+".replaces"), "").(string)
			if replaces != "" {
				_, replacesSet := d.GetOk(replaces)
				if replacesSet {
					appendActionLog(fmt.Sprintf("Reading deprecated/renamed skipping new (for obsolete) field : %s: '%s'.'%s'->'%s'  '%v'\n", typ, name, key, replaces, val))
					continue
				}
			}

			// on failure, if field is deprecated and renamed and set in HCL, try the new name
			deprecatedFor := GetNestedValueOrDefault(attrs, ToKeyPath(key+".deprecated_for"), "").(string)
			if deprecatedFor != "" && val == nil {
				appendActionLog(fmt.Sprintf("Reading deprecated/renamed field : %s: '%s'.'%s'->'%s'  '%v'\n", typ, name, key, deprecatedFor, val))
				_, isSet := d.GetOk(key)
				if isSet {
					_, val, diags = resourceShorelineObjectReadSingleAttr(name, typ, key, attrs, record, stepsJs, d)
				}
			}
			if val == nil {
				// not found, so check for a default value and assume that
				defowlt := GetNestedValueOrDefault(attrs, ToKeyPath(key+".default"), nil)
				if defowlt != nil {
					val = defowlt
					appendActionLog(fmt.Sprintf("Reading (default) %s field: '%s'.'%s' :: %+v\n", typ, name, key, val))
					//appendActionLog(fmt.Sprintf("Reading (default) %s field: '%s'.'%s' steps js::     %+v\n", typ, name, key, stepsJs))
				} else {
					// XXX error?
					appendActionLog(fmt.Sprintf("Reading (failed/empty) %s field: '%s'.'%s' :: %+v\n", typ, name, key, val))
					if typ == "file" {
						if key == "input_file" || key == "md5" {
							continue
						}
					}
					// NOTE: If we don't set a value, TF won't do comparisons or show a diff for this field.
					d.Set(key, nil)
					continue
				}
			}

			appendActionLog(fmt.Sprintf("Reading (updating local state) %s field: '%s'.'%s' :: %+v\n", typ, name, key, val))
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
			case "string_set":
				d.Set(key, CastToArray(val))
			case "string":
				d.Set(key, CastToString(val))
			case "command":
				d.Set(key, CastToString(val))
			case "label":
				d.Set(key, CastToString(val))
			case "time_s":
				d.Set(key, CastToString(val)+"s")
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

		diags = resourceShorelineObjectSetFields(typ, attrs, ctx, d, meta, true, false)
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
