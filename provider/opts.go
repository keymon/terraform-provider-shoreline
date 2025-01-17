// Copyright 2021, Shoreline Software Inc.
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	zstd "github.com/klauspost/compress/zstd"
	"github.com/spf13/viper"
	"io"
	"io/ioutil"
	prand "math/rand"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type CliOpts struct {
	Execute     bool
	Verbose     bool
	StdIn       bool
	Quiet       bool
	CfgFile     string
	HasAuth     bool
	AuthChanged bool
	Url         string
	Token       string
}

const CanonicalUrl = "https://(<backend_node>.)?<customer>.<region>.api.shoreline-<cluster>.io"

var AuthUrl string
var AuthToken string
var RetryLimit int
var DoDebugLog = false
var GlobalOpts = CliOpts{}

var clientAuth *ClientAuth

var AuthConfig = viper.New()

func GetHomeDir() string {
	user, err := user.Current()
	homeDir := "/"
	if err == nil {
		homeDir = user.HomeDir
	}
	return homeDir
}

func GetDotfilePath() string {
	// TODO patch for windows
	home := GetHomeDir()

	// default to "~/"
	thePath := home
	// check if '~/.shoreline/' exists
	shorePath := filepath.Join(home, ".shoreline")
	if _, err := os.Stat(shorePath); !os.IsNotExist(err) {
		thePath = shorePath
	}
	return thePath
}

func getAuthFilename() string {
	return ".ops_auth.yaml"
}

func getAuthUrls() []string {
	urls := []string{}
	auth := AuthConfig.Get("Auth")
	if auth != nil {
		authArr, isArr := auth.([]interface{})
		if isArr {
			for _, obj := range authArr {
				objMap, isMap := obj.(map[interface{}]interface{})
				if isMap {
					url, uOk := objMap["Url"]
					if uOk {
						urlStr, isStr := url.(string)
						if isStr {
							urls = append(urls, urlStr)
						}
					}
				}
			}
		}
	}
	return urls
}

func LoadAuthConfig(GlobalOpts *CliOpts) bool {
	AuthConfig.SetConfigName(getAuthFilename())
	AuthConfig.SetConfigType("yaml")
	//AuthConfig.AddConfigPath(GetHomeDir())
	AuthConfig.AddConfigPath(GetDotfilePath())
	AuthConfig.ReadInConfig() // ignore errors...

	URL := AuthConfig.GetString("Url")
	TOKEN := AuthConfig.GetString("Token")
	//WriteMsg("*** URL: %s, TOKEN:%.16s\n", URL, TOKEN)
	if URL == "" || TOKEN == "" {
		GlobalOpts.HasAuth = false
	} else {
		GlobalOpts.HasAuth = true
		GlobalOpts.Url = URL
		GlobalOpts.Token = TOKEN
	}
	return GlobalOpts.HasAuth
}

func SetAuth(GlobalOpts *CliOpts, Url string, Token string) {
	AuthUrl = Url
	AuthToken = Token
	// set default
	GlobalOpts.Url = Url
	GlobalOpts.Token = Token
	GlobalOpts.HasAuth = true
	GlobalOpts.AuthChanged = true

	//AddAuthEntry(GlobalOpts, Url, Token, false)
}

func selectAuth(GlobalOpts *CliOpts, toUrl string) bool {
	auth := AuthConfig.Get("Auth")
	if auth == nil {
		WriteMsg("No 'Auth' object in config!\n")
		return false
	}
	authArr, isArr := auth.([]interface{})
	if !isArr {
		WriteMsg("Config 'Auth' object is not an array!\n")
		return false
	}
	for i, obj := range authArr {
		objMap, isMap := obj.(map[interface{}]interface{})
		if !isMap {
			WriteMsg("Config 'Auth' element %d is not an object (%T)!\n", i, obj)
			continue
		}
		urlOb, uOk := objMap["Url"]
		url, uStr := urlOb.(string)
		if !(uOk && uStr) {
			continue
		}
		tokenOb, tOk := objMap["Token"]
		token, tStr := tokenOb.(string)
		if !(tOk && tStr) {
			continue
		}
		if url == toUrl {
			SetAuth(GlobalOpts, toUrl, token)
			return true
		}
	}
	return false
}

func PrintAuthWarning() {
	//WriteMsg("Make sure URL and TOKEN are set in the config file: " + "~/.ops_auth.yaml\n")
	WriteMsg("Missing URL and Authorization Token!\n")
	WriteMsg("Get your customer URL from an administrator and enter the command:\n")
	WriteMsg("   'auth " + CanonicalUrl + "' \n")
}

func GetManualAuthMessage(GlobalOpts *CliOpts) string {
	// handle failure with manual copy/paste message (with URL)
	return fmt.Sprintf("ERROR: Automatic authentication token retrieval failed.\n") +
		fmt.Sprintf("Please retry or manually visit the authentication URL:\n") +
		fmt.Sprintf("\t"+GetTokenAuthUrl(GlobalOpts, true)+"\n")
}

func ValidateApiUrl(url string) bool {
	// NOTE: standard URLs are in the form -- "https://<customer>.<region>.api.shoreline-<cluster>.io"
	//   However, users can have custom backends with arbitrary URLs
	//urlRegex := regexp.MustCompile(`^https://\w+\.\w+\.api\.shoreline-\w+\.io$`)
	urlRegex := regexp.MustCompile(`^https://[\.a-z0-9-]+$`)
	if !urlRegex.MatchString(url) {
		WriteMsg("ERROR: Invalid URL to auth! (%s)\n", url)
		WriteMsg("It should be of the form: '" + CanonicalUrl + "' \n")
		return false
	}
	return true
}

// randomly generated key
func GetIdempotencyKey() string {
	// generate 128 high-quality random bytes, return as a hex string
	data := make([]byte, 16)
	hexBytes := make([]byte, 32)
	_, err := rand.Read(data[:])
	if err != nil {
		// Fall back to low-quality psuedo-random numbers.
		// NOTE: This is not ideal, but should be rare.
		if viper.GetBool("debug") {
			WriteMsg("WARNING: High-Quality random numbers unavailable. -- (%v)\n", err.Error())
		}
		prand.Seed(time.Now().UnixNano())
		for i := 0; i < 16; i++ {
			data[i] = byte(prand.Intn(256))
		}
	}
	hex.Encode(hexBytes, data)
	hexStr := string(hexBytes)
	return hexStr
}

func GetInnerErrorStr(errStr string) string {
	msgRegex := regexp.MustCompile(`message: \\".*\\"`)
	loc := msgRegex.FindStringIndex(errStr)
	if loc == nil {
		errStr = strings.Replace(errStr, "\\n", "\n", -1)
		return errStr
	}
	innerStr := errStr[loc[0]+11 : loc[1]-2]
	innerStr = strings.Replace(innerStr, "\\\"", "\"", -1)
	innerStr = strings.Replace(innerStr, "\\\\", "\\", -1)
	return GetInnerErrorStr(innerStr)
}

func GetInnerError(err error) string {
	outer := error.Error(err)
	innerStr := GetInnerErrorStr(outer)
	return innerStr
}

func ExecuteOpCommand(GlobalOpts *CliOpts, expr string) (string, error) {
	if !GlobalOpts.HasAuth {
		return "", fmt.Errorf("No valid auth credentials.")
	} else {
		if clientAuth == nil || clientAuth.BaseURL != GlobalOpts.Url || GlobalOpts.AuthChanged {
			// Auth data is persisted, so that we don't have to re-authorize for every command
			clientAuth = NewClientAuth(GlobalOpts.Url, GlobalOpts.Token, GetIdempotencyKey())
			GlobalOpts.AuthChanged = false
		} else {
			// Fresh Idempotency key for every command.
			clientAuth.ApiKey = GetIdempotencyKey()
		}
		fullExpr := expr
		new_client := NewClient(clientAuth)
		//fix this to be resolved input
		ret, error := new_client.Execute(fullExpr, false)
		if error != nil {
			inner := GetInnerError(error)
			return "", fmt.Errorf(inner)

		} else {
			retStr := string(ret)
			return retStr, nil
		}
	}
}

// Returns base64 data, success/failure, file size, md5 checksum.
func FileToBase64(filename string, skipData bool) (string, bool, int64, string) {
	encoded := ""
	fstat, err := os.Stat(filename)
	if err != nil || fstat.Size() == 0 { // skip non-existent or empty files
		return "", false, 0, ""
	}
	fileLen := fstat.Size()
	file, err := os.Open(filename)
	if err != nil {
		return "", false, 0, ""
	}
	defer file.Close()
	// streaming md5sum
	hash := md5.New()
	_, err = io.Copy(hash, file)
	if err != nil {
		return "", false, 0, ""
	}
	md5Sum := fmt.Sprintf("%x", hash.Sum(nil))

	if !skipData {
		raw, err := ioutil.ReadFile(filename)
		if err != nil {
			//logging.WriteMsgColor(Red, "ERROR: Couldn't read input file: %s !\n", filename)
			return "", false, 0, ""
		}
		// Create a writer that caches compressors.
		// For this operation type we supply a nil Reader.
		var encoder, _ = zstd.NewWriter(nil)

		// Compress a buffer.
		// If you have a destination buffer, the allocation in the call can also be eliminated.
		compressed := encoder.EncodeAll(raw, make([]byte, 0, len(raw)))

		encoded = base64.StdEncoding.EncodeToString(compressed)
	}
	return encoded, true, fileLen, md5Sum
}

////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////
func DownloadFileHttps(src string, dst string, token string) error {
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("Couldn't open local download file '%s'\n", dst)
	}
	defer out.Close()

	resp, err := http.Get(src)
	if err != nil {
		return fmt.Errorf("Couldn't open download url '%s'\n", src)
	}
	defer resp.Body.Close()

	// NOTE: This processes a block at a time, which is important for large files and mem usage
	// TODO download to temp and mv/rm on success/fail
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("Couldn't process download data from url '%s'\n", src)
	}

	return nil
}

func UploadFileHttps(src string, dst string, token string) error {
	file, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("couldn't open local upload file '%s'\n", src)
	}
	defer file.Close()

	stat, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("Couldn't stat file to upload: " + err.Error())
	}
	fileSize := stat.Size()

	reqOb, err := http.NewRequest("PUT", dst, file)
	//reqOb, err := http.NewRequest("POST", dst, file)
	//reqOb, err := http.NewRequest(http.MethodPut, dst, file)
	if err != nil {
		fmt.Printf("Couldn't create upload request object: " + err.Error())
		return fmt.Errorf("Couldn't create upload request object: " + err.Error())
	}
	//reqOb.Header.Set("Content-Type", "application/octet-stream")
	//reqOb.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqOb.ContentLength = fileSize

	response, err := http.DefaultClient.Do(reqOb)
	defer response.Body.Close()
	if err != nil {
		fmt.Printf("Couldn't upload file: " + err.Error())
		return fmt.Errorf("Couldn't upload file: " + err.Error())
	} else {
		fmt.Printf("Uploaded file '%s' (%d bytes) status: %v - %v\n", src, fileSize, response.StatusCode, http.StatusText(response.StatusCode))
	}
	return nil
}

func DeleteFileHttps(dst string, token string) error {
	resp, err := http.Get(dst)
	if err != nil {
		fmt.Printf("Couldn't open delete url '%s'\n", dst)
		return fmt.Errorf("Couldn't open delete url '%s'\n", dst)
	}
	defer resp.Body.Close()

	return nil
}

////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////

//func ExtractPresignedUrl(jsonData []byte) (string, error) {
//	var objects interface{}
//	err := json.Unmarshal(jsonData, &objects)
//	if err != nil {
//		return "", fmt.Errorf("WARNING: Failed to parse JSON from ExtractPresignedUrl().")
//	}
//	url, ok := pretty.GetNestedValueOrDefault(objects, pretty.ToKeyPath("get_file_attribute"), nil).(string)
//	if !ok {
//		return "", fmt.Errorf("WARNING: Missing get_file_attribute key from ExtractPresignedUrl().\n")
//	}
//	return url, nil
//}
//
//func CheckOpCopyUriField(GlobalOpts *opts.CliOpts, symbolName string) (string, bool) {
//	timeout := viper.GetString("background_timeout")
//	pre_command := fmt.Sprintf("%s.uri", symbolName)
//	results_pre, err :=  ExecuteOpCommand(GlobalOpts, pre_command, timeout)
//	if viper.GetBool("debug") {
//		logging.WriteMsgColor(Red, "OpCp legacy test error: %v\n", err)
//	}
//	if err != nil {
//		return "", true
//	}
//	uri, err := ExtractPresignedUrl(results_pre)
//	if err != nil {
//		return "", true
//	}
//	// "get file attribute failed: field does not exist"
//	if strings.Contains(uri, "failed:") || strings.Contains(uri, "field does not exist") {
//		return "", true
//	}
//	// XXX could explicitly check for "s3:"/"gs:"/"https:"(AZ) or "/<symbolName>"
//	return uri, false
//}
//
//func PushOpCopyFileData(GlobalOpts *opts.CliOpts, symbolName string, fileName string) bool {
//	rpath, legacyPush := CheckOpCopyUriField(GlobalOpts, symbolName)
//	timeout := viper.GetString("background_timeout")
//	//pre_command := fmt.Sprintf("%s.uri", symbolName)
//	//results_pre, err :=  ExecuteOpCommand(GlobalOpts, pre_command, timeout)
//	//logging.WriteMsgColor(Red, "OpCp legacy test error: %v\n", err)
//	//if err != nil {
//	//	legacyPush = true
//	//}
//	base64Data := ""
//	if legacyPush {
//		ok := false
//		// deal with source filename being quoted (e.g. spaces in filename)
//		base64Data, ok, _, _ = FileToBase64(fileName, false)
//		if !ok {
//			logging.WriteMsgColor(Red, "OpCp failed failed get base64 data, symbol: %s\n", symbolName)
//			return false
//		}
//	} else {
//		command := fmt.Sprintf("%s.presigned_put", symbolName)
//		results, err :=  ExecuteOpCommand(GlobalOpts, command, timeout)
//		if err != nil {
//			logging.WriteMsgColor(Red, "OpCp failed failed call for presigned URL, symbol: %s\n", symbolName)
//			return false
//		}
//		//rpath, err := ExtractPresignedUrl(results_pre)
//		//if err != nil {
//		//	logging.WriteMsgColor(Red, "OpCp failed failed to extract remote path, symbol: %s\n", symbolName)
//		//	return false
//		//}
//		url, err := ExtractPresignedUrl(results)
//		if err != nil {
//			logging.WriteMsgColor(Red, "OpCp failed failed to extract presigned URL, symbol: %s\n", symbolName)
//			return false
//		}
//		// push to URL
//		logging.WriteMsgColor(Blue, "OpCp uploading to presigned URL, symbol: %s\n   %s\n", symbolName, url)
//		err = UploadFileHttps(fileName, url, "")
//		if err != nil {
//			logging.WriteMsgColor(Red, "OpCp failed failed to extract presigned URL, symbol: %s\n%s\n", symbolName, err.Error())
//			return false
//		}
//		base64Data = fmt.Sprintf(":%s", rpath)
//	}
//	// NOTE: even for S3 files, we need to set file_data to *something*
//	command := fmt.Sprintf("%s.file_data = \"%s\"", symbolName, base64Data)
//	if !HandleOpCommand(GlobalOpts, command, false) {
//		commandTrunc := command
//		if len(commandTrunc) > 200 {
//			commandTrunc = commandTrunc[0:200] + " ..."
//		}
//		logging.WriteMsgColor(Red, "OpCp failed at step: %s\n", commandTrunc)
//		return false
//	}
//	return true
//}
