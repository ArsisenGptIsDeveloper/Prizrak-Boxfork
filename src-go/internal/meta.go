package internal

import (
	"fmt"
	"github.com/legiz-ru/prizrak-box/pkg/constant"
	sysProxy "github.com/legiz-ru/prizrak-box/pkg/sys/proxy"
	"github.com/metacubex/mihomo/hub/executor"
	"github.com/metacubex/mihomo/tunnel"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/legiz-ru/prizrak-box/api/models"
	"github.com/legiz-ru/prizrak-box/pkg/cache"
	"github.com/legiz-ru/prizrak-box/pkg/utils"
	"github.com/metacubex/mihomo/config"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
	plog "github.com/sirupsen/logrus"
)

// Init meta 启动前的初始化
func Init() {
	// 设置工作目录
	C.SetHomeDir(utils.GetUserHomeDir())

	// 设置日志输出目录
	logName := "px-server.log"
	logFilePath := utils.GetUserHomeDir("logs", logName)
	f, err := utils.CreateFileForAppend(logFilePath)
	if err != nil {
		return
	}

	// 组合一下即可，os.Stdout代表标准输出流
	if runtime.GOOS != "windows" {
		// 组合一下即可，os.Stdout代表标准输出流
		multiWriter := io.MultiWriter(os.Stdout, f)
		plog.SetOutput(multiWriter)
	} else {
		plog.SetOutput(f)
	}

	// 设置cache db
	db := cache.GetDBInstance()
	if db == nil {
		os.Exit(1)
	}
	cache.GetMetaDB()

	// 输出日志
	log.Infoln("[CacheDB] initialized")
	log.Infoln("[HomePath] is %s", utils.GetUserHomeDir())

	// 修改权限
	pathTemp := utils.GetUserHomeDir("logs", "px-client.log")
	_ = utils.SetPermissions(pathTemp)
	pathTemp = utils.GetUserHomeDir("px-electron.db")
	_ = utils.SetPermissions(pathTemp)
	pathTemp = utils.GetUserHomeDir("px-electron.db/config.json")
	pathTempDst := utils.GetUserHomeDir("px-electron.db/config_temp.json")
	_ = utils.ModifyFilePermissions(pathTemp, pathTempDst)
	log.Infoln("[Permission] is ok")

	// 释放资源文件
	_, _ = utils.SaveFile(utils.GetUserHomeDir("geoip.metadb"), GeoIp)
	_, _ = utils.SaveFile(utils.GetUserHomeDir("GeoSite.dat"), GeoSite)
	_, _ = utils.SaveFile(utils.GetUserHomeDir("ASN.mmdb"), ASN)

	// 释放大模型
	bin := utils.GetUserHomeDir("Model.bin")
	if !utils.FileExists(bin) {
		_, _ = utils.SaveFile(bin, ModelBin)
	}

	GeoIp = nil
	GeoSite = nil
	ASN = nil
	ModelBin = nil

	EnsureBuiltinTemplates()
}

var NowConfig *config.Config
var havaStartCore bool
var StartLock = sync.Mutex{}

// startCore 函数用于启动核心功能
func startCore(profiles []models.Profile, reload bool) {
	if len(profiles) == 0 {
		return
	}

	primary := profiles[0]

	// 获取规则分组
	useTemplate, templateId, templateBuf := getTemplate(primary)

	// 获取配置文件
	providerBuf, err := os.ReadFile(utils.GetUserHomeDir(primary.Path))
	if err != nil {
		log.Warnln("Read config error: %s", err.Error())
		return
	}

	// 解析配置文件1
	rawCfg, err := config.UnmarshalRawConfig(providerBuf)
	if err != nil {
		log.Warnln("Unmarshal config error: %s", err.Error())
		return
	}

	mergeRawConfigProfiles(rawCfg, profiles[1:])

	// 统一规则模板
	if useTemplate || len(rawCfg.Rule) == 0 {
		provider := rawCfg.ProxyProvider
		proxy := rawCfg.Proxy
		rawCfg, _ = config.UnmarshalRawConfig(templateBuf)
		changeProvidersPath("template", templateId, rawCfg)
		if len(provider) > 0 {
			if len(rawCfg.ProxyProvider) > 0 {
				for key, value := range provider {
					rawCfg.ProxyProvider[key] = value
				}
			} else {
				rawCfg.ProxyProvider = provider
			}
		}
		if len(rawCfg.ProxyProvider) > 1 {
			for key, value := range rawCfg.ProxyProvider {
				value["override"] = map[string]string{"additional-suffix": "-" + key}
			}
		}
		if len(proxy) > 0 {
			if len(rawCfg.Proxy) > 0 {
				rawCfg.Proxy = append(rawCfg.Proxy, proxy...)
			} else {
				rawCfg.Proxy = proxy
			}
		}
		rawCfg.Rule = mergeRulesBeforeMatch(rawCfg.Rule, mergedRules)
	}

	// Prizrak-Box 默认配置
	rawCfg.Port = 0
	rawCfg.SocksPort = 0
	rawCfg.TProxyPort = 0
	rawCfg.RedirPort = 0
	rawCfg.ExternalController = ""
	rawCfg.ExternalUI = ""
	rawCfg.ExternalUIURL = ""
	rawCfg.Tun.DNSHijack = []string{"any:53"}
	rawCfg.Tun.AutoRoute = true
	rawCfg.Tun.AutoDetectInterface = true
	rawCfg.Tun.Device = "Prizrak"
	rawCfg.UnifiedDelay = true

	// 从数据库中获取 mihomo 配置,进行 rawCfg 赋值
	var mi models.Mihomo
	_ = cache.Get(constant.Mihomo, &mi)
	if mi.BindAddress == "" {
		mi = models.Mihomo{
			Mode:        "rule",
			Proxy:       false,
			Tun:         false,
			Port:        9697,
			BindAddress: "127.0.0.1",
			Stack:       "Mixed",
			Dns:         false,
			Ipv6:        false,
		}
	}
	rawCfg.Mode = tunnel.ModeMapping[mi.Mode]
	rawCfg.AllowLan = true
	rawCfg.MixedPort = mi.Port
	rawCfg.BindAddress = mi.BindAddress
	rawCfg.Tun.Stack = C.StackTypeMapping[strings.ToLower(mi.Stack)]
	rawCfg.IPv6 = mi.Ipv6

	applyProcessBypassRules(rawCfg, mi.Tun)

	// 保存规则数
	_ = cache.Put("Rule_No", len(rawCfg.Rule))

	// 解析配置文件2
	NowConfig, err = config.ParseRawConfig(rawCfg)
	if err != nil {
		log.Errorln("ParseRawConfig error: %v", err)
		return
	}

	// 覆盖dns
	if mi.Dns {
		var dns models.Dns
		_ = cache.Get(constant.Dns, &dns)

		if dns.Content == "" {
			dns.Content = DefaultDNS
		}

		cfg, _ := executor.ParseWithBytes([]byte(dns.Content))
		NowConfig.DNS = cfg.DNS
	}

	// 应用配置
	if reload {
		NowConfig.General.Tun.Enable = mi.Tun
	} else {
		// 检测端口占用
		err = utils.IsPortAvailable(mi.BindAddress, mi.Port)
		if err != nil {
			log.Errorln("IsPortAvailable error: %v", err)
			mi.Port, _ = utils.GetRandomPort(mi.BindAddress)
			NowConfig.General.MixedPort = mi.Port
		}

		// 初次加载不能开启tun,不然在windows上会崩
		NowConfig.General.Tun.Enable = false
	}

	// 激活配置
	go executor.ApplyConfig(NowConfig, !reload)

	// 代理开启
	if mi.Proxy {
		_ = sysProxy.EnableProxy(mi.BindAddress, mi.Port)
	}
	// 存储配置
	_ = cache.Put(constant.Mihomo, mi)
	// 更新启动标志
	havaStartCore = true
}

func mergeRawConfigProfiles(rawCfg *config.RawConfig, profiles []models.Profile) {
	if rawCfg == nil || len(profiles) == 0 {
		return
	}

	for _, profile := range profiles {
		providerBuf, err := os.ReadFile(utils.GetUserHomeDir(profile.Path))
		if err != nil {
			log.Warnln("Read profile config error: %s", err.Error())
			continue
		}

		otherCfg, err := config.UnmarshalRawConfig(providerBuf)
		if err != nil {
			log.Warnln("Unmarshal profile config error: %s", err.Error())
			continue
		}

		if len(otherCfg.ProxyProvider) > 0 {
			if rawCfg.ProxyProvider == nil {
				rawCfg.ProxyProvider = map[string]map[string]any{}
			}
			for key, value := range otherCfg.ProxyProvider {
				mergedKey := key
				if _, exists := rawCfg.ProxyProvider[mergedKey]; exists {
					mergedKey = fmt.Sprintf("%s_%s", profile.Id, key)
				}
				rawCfg.ProxyProvider[mergedKey] = value
			}
		}

		if len(otherCfg.Proxy) > 0 {
			rawCfg.Proxy = append(rawCfg.Proxy, otherCfg.Proxy...)
		}

		if len(otherCfg.Rule) > 0 {
			rawCfg.Rule = append(rawCfg.Rule, otherCfg.Rule...)
		}
	}
}

func applyProcessBypassRules(rawCfg *config.RawConfig, tunEnabled bool) {
	if rawCfg == nil {
		return
	}

	var bypass models.ProcessBypass
	_ = cache.Get(constant.ProcessBypass, &bypass)

	if !tunEnabled || !bypass.Enable || len(bypass.Processes) == 0 {
		return
	}

	processRules := make([]string, 0, len(bypass.Processes))
	for _, item := range bypass.Processes {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		processRules = append(processRules, fmt.Sprintf("PROCESS-NAME,%s,DIRECT", name))
	}

	if len(processRules) == 0 {
		return
	}

	matchIndex := -1
	for i, rule := range rawCfg.Rule {
		normalized := strings.ToUpper(strings.TrimSpace(rule))
		if strings.HasPrefix(normalized, "MATCH,") {
			matchIndex = i
			break
		}
	}

	if matchIndex == -1 {
		rawCfg.Rule = append(rawCfg.Rule, processRules...)
		return
	}

	result := make([]string, 0, len(rawCfg.Rule)+len(processRules))
	result = append(result, rawCfg.Rule[:matchIndex]...)
	result = append(result, processRules...)
	result = append(result, rawCfg.Rule[matchIndex:]...)
	rawCfg.Rule = result
}

// 获取统一规则分组模板
func getTemplate(profile models.Profile) (bool, string, []byte) {
	// 默认模版ID
	defaultId := fmt.Sprintf("%s%d", constant.PrefixTemplate, 0)

	// 优先启用个性模板
	var template models.Template
	if profile.Template != "" {
		if profile.Template == "m0" {
			return false, defaultId, Template_0
		}
		_ = cache.Get(profile.Template, &template)
	}
	if template.Path != "" {
		body, err := utils.ReadFile(utils.GetUserHomeDir(template.Path))
		if err == nil {
			return true, template.Id, []byte(body)
		}
	}

	// 其次启用通用模板
	var list []models.Template
	_ = cache.GetList(constant.PrefixTemplate, &list)
	for _, m := range list {
		if m.Selected {
			template = m
			break
		}
	}
	if template.Path != "" {
		body, err := utils.ReadFile(utils.GetUserHomeDir(template.Path))
		if err == nil {
			return true, template.Id, []byte(body)
		}
	}

	// 最后返回默认模板
	return false, defaultId, Template_0
}

// SwitchProfile 切换配置
func SwitchProfile(reload bool) {
	StartLock.Lock()
	defer StartLock.Unlock()

	// 获取切换配置
	var profiles []models.Profile
	_ = cache.GetList(constant.PrefixProfile, &profiles)

	if len(profiles) == 0 {
		return
	}

	selectedProfiles := make([]models.Profile, 0)
	for _, p := range profiles {
		if p.Selected {
			selectedProfiles = append(selectedProfiles, p)
		}
	}

	if len(selectedProfiles) == 0 {
		profiles[0].Selected = true
		_ = cache.Put(profiles[0].Id, profiles[0])
		selectedProfiles = append(selectedProfiles, profiles[0])
	}

	if !havaStartCore {
		reload = false
	}

	startCore(selectedProfiles, reload)
}
