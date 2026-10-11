// Adapted from mmqz/cpa-multi-plugins, MIT; see NOTICE.md.
package upstream

const (
	AgentHost       = "https://trae-api-cn.mchost.guru"
	UgHost          = "https://api.trae.cn"
	OAuthHost       = "https://api.trae.com.cn"
	ConsoleHost     = "https://www.trae.cn"
	ClientID        = "ono9krqynydwx5" // non-solo (Trae Code CN)
	AppID           = "6eefa01c-1036-4c7e-9ca5-d891f63bfcd8"
	IdeVersion      = "0.1.61"
	IdeVersionCode  = "20260820"
	DeviceBrand     = "83DG"
	OSVersion       = "Windows 11 Pro"
	Function        = "inline_chat" // dead value, kept for compat only
	EpChat          = "/api/agent/v3/llm_utils_chat"
	EpModels        = "/api/ide/v1/get_detail_param"
	EpExchange      = "/cloudide/api/v3/trae/oauth/ExchangeToken"
	EpUserInfo      = "/cloudide/api/v3/trae/GetUserInfo"
	EpCheckLogin    = "/cloudide/api/v3/trae/CheckLogin"
	EpCheckinStatus = "/trae/api/v2/ug/checkin_credits/status"
	EpCheckinClaim  = "/trae/api/v2/ug/checkin_credits/claim"
	EpEntUsage      = "/trae/api/v2/pay/ide_user_ent_usage"
	EpPayStatus     = "/trae/api/v1/pay/ide_user_pay_status"
)
