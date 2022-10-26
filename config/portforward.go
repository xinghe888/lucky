package config

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/gdy666/lucky/socketproxy"
	"github.com/gdy666/lucky/thirdlib/gdylib/logsbuffer"
	"github.com/sirupsen/logrus"
)

type PortForwardsConfigure struct {
	PortForwardsLimit                  int64 `json:"PortForwardsLimit"`                  //全局端口转发数量限制
	TCPPortforwardMaxConnections       int64 `json:"TCPPortforwardMaxConnections"`       //端口转发全局TCP并发链接数限制
	UDPReadTargetDataMaxgoroutineCount int64 `json:"UDPReadTargetDataMaxgoroutineCount"` //端口转发全局UDP读取目标地址数据协程数限制
}

type PortForwardsRule struct {
	Name              string                       `json:"Name"`
	Key               string                       `json:"Key"`
	Enable            bool                         `json:"Enable"`
	ForwardTypes      []string                     `json:"ForwardTypes"`
	ListenAddress     string                       `json:"ListenAddress"`
	ListenPorts       string                       `json:"ListenPorts"`
	TargetAddressList []string                     `json:"TargetAddressList"`
	TargetPorts       string                       `json:"TargetPorts"`
	Options           socketproxy.RelayRuleOptions `json:"Options"`
	ReverseProxyList  *[]socketproxy.Proxy         `json:"-"`

	logsBuffer                 *logsbuffer.LogsBuffer
	logrus                     *logrus.Logger
	LogLevel                   int  `json:"LogLevel"`           //日志输出级别
	LogOutputToConsole         bool `json:"LogOutputToConsole"` //日志输出到终端
	AccessLogMaxNum            int  `json:"AccessLogMaxNum"`
	WebListShowLastLogMaxCount int  `json:"WebListShowLastLogMaxCount"` //前端列表显示最新日志最大条数
}

func (r *PortForwardsRule) ProxyCount() int {
	if r.ReverseProxyList == nil {
		return 0
	}
	return len(*r.ReverseProxyList)
}

func (r *PortForwardsRule) StartAllProxys() {
	if r.ReverseProxyList == nil {
		return
	}
	for i := range *r.ReverseProxyList {

		(*r.ReverseProxyList)[i].StartProxy()
	}
}

func (r *PortForwardsRule) GetLastLogs(maxCount int) []any {
	return r.GetLogsBuffer().GetLastLogs(WebLogConvert, maxCount)
}

func (r *PortForwardsRule) Fire(entry *logrus.Entry) error {
	if !r.LogOutputToConsole {
		return nil
	}
	s, _ := entry.String()
	log.Print(s)
	return nil
}

func (r *PortForwardsRule) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (r *PortForwardsRule) GetLogrus() *logrus.Logger {
	if r.logrus == nil {
		r.logrus = logrus.New()
		r.logrus.SetLevel(logrus.Level(r.LogLevel))
		r.logrus.SetOutput(r.GetLogsBuffer())
		r.logrus.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat:   "2006-01-02 15:04:05",
			DisableTimestamp:  true,
			DisableHTMLEscape: true,
			DataKey:           "ExtInfo",
		})
		r.logrus.AddHook(r)
	}
	return r.logrus
}

func (r *PortForwardsRule) GetLogsBuffer() *logsbuffer.LogsBuffer {
	if r.logsBuffer == nil {
		r.logsBuffer = logsbuffer.CreateLogbuffer("portforward:"+r.Key, r.AccessLogMaxNum)
	}
	return r.logsBuffer
}

func (r *PortForwardsRule) StopAllProxys() {
	if r.ReverseProxyList == nil {
		return
	}
	for i := range *r.ReverseProxyList {
		(*r.ReverseProxyList)[i].StopProxy()
	}
}

// TidyReverseProxyCache 整理端口转发日志缓存
func TidyPortforwardLogsCache() {
	ruleList := GetPortForwardsRuleList()
	var keyListBuffer strings.Builder
	for _, rule := range ruleList {
		keyListBuffer.WriteString(rule.Key)
		keyListBuffer.WriteString(",")
	}

	keyListStr := keyListBuffer.String()
	logsbuffer.LogsBufferStoreMu.Lock()
	defer logsbuffer.LogsBufferStoreMu.Unlock()

	var needDeleteKeys []string

	for k := range logsbuffer.LogsBufferStore {
		if !strings.HasPrefix(k, "portforward:") {
			continue
		}

		if len(k) <= 13 {
			continue
		}

		if !strings.Contains(keyListStr, k[12:]) {
			needDeleteKeys = append(needDeleteKeys, k)
		}
	}

	for i := range needDeleteKeys {
		delete(logsbuffer.LogsBufferStore, needDeleteKeys[i])
	}

}

func (r *PortForwardsRule) InitProxyList() error {
	listenPorts, err := PortsStrToIList(r.ListenPorts)
	if err != nil {
		return err
	}
	targetPorts, err := PortsStrToIList(r.TargetPorts)
	if err != nil {
		return err
	}
	if len(listenPorts) != len(targetPorts) {
		return fmt.Errorf("端口个数不一致")
	}
	var proxyList []socketproxy.Proxy
	for i := range r.ForwardTypes {
		for j := range listenPorts {
			p, err := socketproxy.CreateProxy(r.GetLogrus(), r.ForwardTypes[i],
				r.ListenAddress,
				r.TargetAddressList,
				listenPorts[j],
				targetPorts[j],
				&r.Options)
			if err != nil {
				return err
			}
			proxyList = append(proxyList, p)
		}
	}

	r.ReverseProxyList = &proxyList
	return nil
}

func PortsCheck(ports1Str, ports2Str string) (bool, error) {
	ports1, err := PortsStrToIList(ports1Str)
	if err != nil {
		return false, err
	}
	ports2, err := PortsStrToIList(ports2Str)
	if err != nil {
		return false, err
	}
	if len(ports1) != len(ports2) {
		return false, fmt.Errorf("端口个数不一致")
	}
	return true, nil
}

// portsStrToIList
func PortsStrToIList(portsStr string) (ports []int, err error) {
	if portsStr == "" {
		return
	}
	if strings.Contains(portsStr, ",") {
		tmpStrList := strings.Split(portsStr, ",")
		for i := range tmpStrList {
			tps, e := PortsStrToIList(tmpStrList[i])
			if e != nil {
				err = fmt.Errorf("端口字符串处理出错:%s", e.Error())
				return
			}
			ports = append(ports, tps...)
		}

		return
	}

	portsStrList := strings.Split(portsStr, "-")
	if len(portsStrList) > 2 {
		err = fmt.Errorf("端口%s格式有误", portsStr)
		return
	}

	if len(portsStrList) == 1 { //single listen port
		listenPort, e := portStrToi(portsStrList[0])
		if e != nil {
			err = fmt.Errorf("端口格式有误!%s", e.Error())
			return
		}
		ports = append(ports, listenPort)
	}

	if len(portsStrList) == 2 {
		minListenPort, e := portStrToi(portsStrList[0])
		if e != nil {
			err = fmt.Errorf("端口格式有误!%s", portsStrList[0])
			return
		}
		maxListenPort, e := portStrToi(portsStrList[1])
		if e != nil {
			err = fmt.Errorf("端口格式有误!%s", portsStrList[1])
			return
		}

		if maxListenPort <= minListenPort {
			err = fmt.Errorf("前一个端口[%d]要小于后一个端口[%d]", minListenPort, maxListenPort)
			return
		}
		i := minListenPort
		for {
			if i > maxListenPort {
				break
			}
			ports = append(ports, i)
			i++
		}
	}

	return
}

func portStrToi(portStr string) (int, error) {
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("端口格式有误:%s", err.Error())
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("端口[%d]超出范围", port)
	}
	return port, nil
}

func PortForwardsRuleListInit() {
	programConfigureMutex.RLock()
	defer programConfigureMutex.RUnlock()
	var err error
	for i := range programConfigure.PortForwardsRuleList {
		err = programConfigure.PortForwardsRuleList[i].InitProxyList()
		if err != nil {
			log.Printf("InitProxyList error:%s\n", err.Error())
		}
		if programConfigure.PortForwardsRuleList[i].Enable {
			programConfigure.PortForwardsRuleList[i].StartAllProxys()
		}
	}
}

func GetPortForwardsRuleList() []PortForwardsRule {
	programConfigureMutex.RLock()
	defer programConfigureMutex.RUnlock()

	var resList []PortForwardsRule

	for i := range programConfigure.PortForwardsRuleList {
		r := programConfigure.PortForwardsRuleList[i]
		resList = append(resList, r)
	}
	return resList
}

func GetPortForwardsGlobalProxyCount() int {
	programConfigureMutex.RLock()
	defer programConfigureMutex.RUnlock()
	count := 0
	for i := range programConfigure.PortForwardsRuleList {
		count += programConfigure.PortForwardsRuleList[i].ProxyCount()
	}
	return count
}

func GetPortForwardsGlobalProxyCountExcept(key string) int {
	programConfigureMutex.RLock()
	defer programConfigureMutex.RUnlock()
	count := 0
	for i := range programConfigure.PortForwardsRuleList {
		if key == programConfigure.PortForwardsRuleList[i].Key {
			continue
		}
		count += programConfigure.PortForwardsRuleList[i].ProxyCount()
	}
	return count
}

func GetPortForwardsRuleByKey(key string) *PortForwardsRule {
	programConfigureMutex.RLock()
	defer programConfigureMutex.RUnlock()
	index := -1

	for i := range programConfigure.PortForwardsRuleList {
		if programConfigure.PortForwardsRuleList[i].Key == key {
			index = i
			break
		}
	}
	if index == -1 {
		return nil
	}
	res := programConfigure.PortForwardsRuleList[index]
	return &res
}

func StopAllSocketProxysByRuleKey(key string) error {
	programConfigureMutex.RLock()
	defer programConfigureMutex.RUnlock()
	index := -1

	for i := range programConfigure.PortForwardsRuleList {
		if programConfigure.PortForwardsRuleList[i].Key == key {
			index = i
			break
		}
	}
	if index == -1 {
		return fmt.Errorf("找不到key:%s对应的规则", key)
	}
	programConfigure.PortForwardsRuleList[index].StopAllProxys()
	return nil
}

func StartAllSocketProxysByRuleKey(key string) error {
	programConfigureMutex.RLock()
	defer programConfigureMutex.RUnlock()
	index := -1

	for i := range programConfigure.PortForwardsRuleList {
		if programConfigure.PortForwardsRuleList[i].Key == key {
			index = i
			break
		}
	}
	if index == -1 {
		return fmt.Errorf("找不到key:%s对应的规则", key)
	}
	programConfigure.PortForwardsRuleList[index].StartAllProxys()
	return nil
}

func PortForwardsRuleListAdd(r *PortForwardsRule) error {
	programConfigureMutex.Lock()
	defer programConfigureMutex.Unlock()
	r.Enable = true
	programConfigure.PortForwardsRuleList = append(programConfigure.PortForwardsRuleList, *r)
	return Save()
}

func PortForwardsRuleListDelete(key string) error {
	programConfigureMutex.Lock()
	defer programConfigureMutex.Unlock()

	index := -1

	for i := range programConfigure.PortForwardsRuleList {
		if programConfigure.PortForwardsRuleList[i].Key == key {
			index = i
			break
		}
	}

	if index == -1 {
		return fmt.Errorf("找不到需要删除的端口转发规则")
	}

	programConfigure.PortForwardsRuleList[index].StopAllProxys()

	programConfigure.PortForwardsRuleList = DeletePortForwardsRuleListSlice(programConfigure.PortForwardsRuleList, index)
	return Save()
}

func EnablePortForwardsRuleByKey(key string, enable bool) error {
	programConfigureMutex.Lock()
	defer programConfigureMutex.Unlock()
	index := -1

	for i := range programConfigure.DDNSTaskList {
		if programConfigure.PortForwardsRuleList[i].Key == key {
			index = i
			break
		}
	}
	if index == -1 {
		return fmt.Errorf("开关端口转发规则失败,key查找失败")
	}

	if enable {
		programConfigure.PortForwardsRuleList[index].StartAllProxys()
	} else {
		programConfigure.PortForwardsRuleList[index].StopAllProxys()
	}

	programConfigure.PortForwardsRuleList[index].Enable = enable
	return Save()
}

func UpdatePortForwardsRuleToPortForwardsRuleList(key string, r *PortForwardsRule) error {
	programConfigureMutex.Lock()
	defer programConfigureMutex.Unlock()
	index := -1

	for i := range programConfigure.PortForwardsRuleList {
		if programConfigure.PortForwardsRuleList[i].Key == key {
			index = i
			break
		}
	}

	if index == -1 {
		return fmt.Errorf("找不到需要更新的端口转发规则")
	}

	programConfigure.PortForwardsRuleList[index] = *r
	return Save()
}

func DeletePortForwardsRuleListSlice(a []PortForwardsRule, deleteIndex int) []PortForwardsRule {
	j := 0
	for i := range a {
		if i != deleteIndex {
			a[j] = a[i]
			j++
		}
	}
	return a[:j]
}
