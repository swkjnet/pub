package pub

import (
	"bytes"
	"compress/zlib"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

//压缩
func DoZlibCompress(src []byte) []byte {
	var in bytes.Buffer
	w := zlib.NewWriter(&in)
	w.Write(src)
	w.Close()
	return in.Bytes()
}

//获取配置文件
func GetConfig(strfile string, v interface{}) error {
	file, err := os.OpenFile(strfile, os.O_RDWR, 0660)
	if err != nil {
		return err
	}
	buf := make([]byte, 655350)
	len, _ := file.Read(buf)
	file.Close()
	jerr := json.Unmarshal(buf[:len], v)
	if jerr != nil {
		return jerr
	}
	return nil
}

//更新配置文件
func UpdConfig(strfile string, data []byte, v interface{}) bool {
	err := json.Unmarshal(data, v)
	if err != nil {
		PrintLog("error json unmarshal:", string(data))
		return false
	}
	configBuf, _ := json.Marshal(v)
	os.Remove("./config.json")
	file, err := os.OpenFile("./config.json", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0660)
	if err != nil {
		PrintLog("write to config file err:", err.Error())
		return false
	}
	configStr := strings.Join(strings.Split(string(configBuf), ","), ",\n")
	configStr = strings.Join(strings.Split(configStr, "{"), "{\n")
	configStr = strings.Join(strings.Split(configStr, "}"), "\n}")
	file.Write([]byte(configStr))
	file.Close()
	return true
}

//随机变量
var r = rand.New(rand.NewSource(time.Now().UnixNano()))

//随机数
func RandInt(min, max int) int {
	if min >= max || min == 0 || max == 0 {
		return max
	}
	return r.Intn(max-min+1) + min
}

//类型判定
var sliceOfInts = reflect.TypeOf([]int(nil))
var sliceOfStrings = reflect.TypeOf([]string(nil))

//是否是结构体
func IsStructPtr(t reflect.Type) bool {
	return t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Struct
}

//对象转url参数
//返回值为编码后以及编码前的
func ToUrlParams(obj interface{}) (string, string) {
	if obj == nil {
		return "", ""
	}
	objT := reflect.TypeOf(obj)
	objV := reflect.ValueOf(obj)
	if !IsStructPtr(objT) {
		return "", ""
	}
	res := make([]string, 0)
	orires := make([]string, 0)
	objT = objT.Elem()
	objV = objV.Elem()
	for i := 0; i < objT.NumField(); i++ {
		fieldV := objV.Field(i)
		if !fieldV.CanSet() {
			continue
		}
		fieldT := objT.Field(i)
		tag := fieldT.Tag.Get("json")
		if tag == "-" {
			continue
		} else if tag == "" {
			tag = fieldT.Name
		}
		res = append(res, fmt.Sprint(tag, "=", url.QueryEscape(fmt.Sprint(fieldV.Interface()))))
		orires = append(orires, fmt.Sprint(tag, "=", fieldV.Interface()))
	}
	return strings.Join(res, "&"), strings.Join(orires, "&")
}

// ParseForm will parse form values to struct via tag.
func ParseForm(form url.Values, obj interface{}) error {
	objT := reflect.TypeOf(obj)
	objV := reflect.ValueOf(obj)
	if !IsStructPtr(objT) {
		return fmt.Errorf("%v must be a struct pointer", obj)
	}
	objT = objT.Elem()
	objV = objV.Elem()
	for i := 0; i < objT.NumField(); i++ {
		fieldV := objV.Field(i)
		if !fieldV.CanSet() {
			continue
		}
		fieldT := objT.Field(i)
		tags := strings.Split(fieldT.Tag.Get("json"), ",")
		var tag string
		if len(tags) == 0 || len(tags[0]) == 0 {
			tag = fieldT.Name
		} else if tags[0] == "-" {
			continue
		} else {
			tag = tags[0]
		}

		value := form.Get(tag)
		if len(value) == 0 {
			continue
		}
		switch fieldT.Type.Kind() {
		case reflect.Bool:
			if strings.ToLower(value) == "on" || strings.ToLower(value) == "1" || strings.ToLower(value) == "yes" {
				fieldV.SetBool(true)
				continue
			}
			if strings.ToLower(value) == "off" || strings.ToLower(value) == "0" || strings.ToLower(value) == "no" {
				fieldV.SetBool(false)
				continue
			}
			b, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			fieldV.SetBool(b)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			x, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return err
			}
			fieldV.SetInt(x)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			x, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return err
			}
			fieldV.SetUint(x)
		case reflect.Float32, reflect.Float64:
			x, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return err
			}
			fieldV.SetFloat(x)
		case reflect.Interface:
			fieldV.Set(reflect.ValueOf(value))
		case reflect.String:
			fieldV.SetString(value)
		case reflect.Struct:
			switch fieldT.Type.String() {
			case "time.Time":
				format := time.RFC3339
				if len(tags) > 1 {
					format = tags[1]
				}
				t, err := time.ParseInLocation(format, value, time.Local)
				if err != nil {
					return err
				}
				fieldV.Set(reflect.ValueOf(t))
			}
		case reflect.Slice:
			if fieldT.Type == sliceOfInts {
				formVals := form[tag]
				fieldV.Set(reflect.MakeSlice(reflect.SliceOf(reflect.TypeOf(int(1))), len(formVals), len(formVals)))
				for i := 0; i < len(formVals); i++ {
					val, err := strconv.Atoi(formVals[i])
					if err != nil {
						return err
					}
					fieldV.Index(i).SetInt(int64(val))
				}
			} else if fieldT.Type == sliceOfStrings {
				formVals := form[tag]
				fieldV.Set(reflect.MakeSlice(reflect.SliceOf(reflect.TypeOf("")), len(formVals), len(formVals)))
				for i := 0; i < len(formVals); i++ {
					fieldV.Index(i).SetString(formVals[i])
				}
			}
		}
	}
	return nil
}

//获取空闲端口号
func GetFreePort(num int) ([]int, error) {
	index := 0
	res := make([]int, 0)
	for index < num {
		index++
		l, err := net.Listen("tcp", ":0")
		if err != nil {
			return nil, err
		}
		defer l.Close()
		if v, ok := l.Addr().(*net.TCPAddr); ok {
			res = append(res, v.Port)
		} else {
			return nil, errors.New("*net.tcpaddr assertion failure.")
		}
	}
	return res, nil
}

//获取内网IP
func GetInternal() ([]string, error) {
	var res []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	for _, v := range addrs {
		if ipnet, ok := v.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				res = append(res, ipnet.IP.String())
			}
		}
	}
	if len(res) != 0 {
		return res, nil
	}
	return nil, errors.New("not find internal IP")
}

//获取外网IP
func GetExternal() (string, error) {
	httpclient := http.Client{Timeout: 2 * time.Second}
	resp, err := httpclient.Get("http://myexternalip.com/raw")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	res, err2 := ioutil.ReadAll(resp.Body)
	if err2 != nil {
		return "", err2
	}
	r, err3 := regexp.Compile("\\s")
	if err3 != nil {
		return "", err3
	}
	res = r.ReplaceAll(res, []byte(""))
	return string(res), nil
}

//httpGet请求
func HttpsGet(urlStr string, timeout time.Duration) ([]byte, time.Duration, error) {
	t := time.Now()
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	client := http.Client{Timeout: timeout, Transport: tr}
	resp, err := client.Get(urlStr)
	if err != nil {
		return []byte("overtime"), 0, err
	}
	buf, err := ioutil.ReadAll(resp.Body)
	return buf, time.Since(t), err
}

//获取带base64_sha1签名url地址
func GetSigUrl(urlStr string, uri string, key string, data interface{}) string {
	param, oriparam := ToUrlParams(data)
	ori := fmt.Sprint("GET", "&", url.QueryEscape(uri), "&", url.QueryEscape(oriparam))
	sig := base64_sha1(ori, key)
	return fmt.Sprint(urlStr, uri, "?", param, "&sig=", url.QueryEscape(sig))
}

//sigurl请求
func HttpsSigURL(urlStr string, uri string, key string, data interface{}, timeout time.Duration) ([]byte, time.Duration, error) {
	return HttpsGet(GetSigUrl(urlStr, uri, key, data), timeout)
}

//获取url参数
func getHttpParams(oriUrl string, name string) string {
	r, _ := regexp.Compile("(\\?|#|&)" + name + "=([^&#]*)(&|#|$)")
	arr := r.FindStringSubmatch(oriUrl)
	if len(arr) > 3 {
		return arr[2]
	}
	return ""
}

//发送报警信息
func SendAlarm(desc string) {
	cmd := exec.Command("/bin/sh", "-c", "cagent_tools alarm '"+desc+"'")
	err := cmd.Run()
	if err != nil {
		PrintLog("cmd.Run:", err.Error())
	}
}
