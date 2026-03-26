package controller

import (
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type clawProfileData struct {
	ID           int    `json:"id"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	Avatar       string `json:"avatar,omitempty"`
	Plan         string `json:"plan"`
	PlanName     string `json:"planName"`
	Quota        int    `json:"quota"`
	UsedQuota    int    `json:"used_quota"`
	RequestCount int    `json:"request_count"`
}

type clawModelConfig struct {
	DefaultModel string   `json:"default_model"`
	ModelList    []string `json:"model_list"`
	Mem0Enabled  bool     `json:"mem0_enabled"`
}

type clawBootstrapData struct {
	User           clawProfileData `json:"user"`
	Profile        clawProfileData `json:"profile"`
	ModelConfig    clawModelConfig `json:"model_config"`
	Models         clawModelConfig `json:"models"`
	APIBase        string          `json:"api_base"`
	APIBaseURL     string          `json:"api_base_url"`
	BackendURL     string          `json:"backend_url"`
	BackendURLAlt  string          `json:"backendUrl"`
	ClawBase       string          `json:"claw_base"`
	FrontendUI     string          `json:"frontend_ui"`
	AccessToken    string          `json:"access_token,omitempty"`
	HasAccessToken bool            `json:"has_access_token"`
}

func getClawBackendBaseURL() string {
	baseURL := strings.TrimSpace(os.Getenv("POCO_CLAW_BACKEND_URL"))
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8000"
	}
	return strings.TrimSuffix(baseURL, "/")
}

func listClawUserModels(c *gin.Context, userID int) ([]string, error) {
	userCache, err := model.GetUserCache(userID)
	if err != nil {
		return nil, err
	}

	modelSet := make(map[string]struct{})
	addModel := func(modelName string) {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			return
		}
		endpointTypes := model.GetModelSupportEndpointTypes(modelName)
		if len(endpointTypes) > 0 && !supportsClawChat(endpointTypes) {
			return
		}
		modelSet[modelName] = struct{}{}
	}

	modelLimitEnable := common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled)
	if modelLimitEnable {
		s, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
		if ok {
			if tokenModelLimit, ok := s.(map[string]bool); ok {
				for allowModel := range tokenModelLimit {
					addModel(allowModel)
				}
			}
		}
	} else {
		tokenGroup := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
		switch tokenGroup {
		case "auto":
			for _, group := range service.GetUserAutoGroup(userCache.Group) {
				for _, modelName := range model.GetGroupEnabledModels(group) {
					addModel(modelName)
				}
			}
		case "":
			groups := service.GetUserUsableGroups(userCache.Group)
			for group := range groups {
				for _, modelName := range model.GetGroupEnabledModels(group) {
					addModel(modelName)
				}
			}
		default:
			for _, modelName := range model.GetGroupEnabledModels(tokenGroup) {
				addModel(modelName)
			}
		}
	}

	models := make([]string, 0, len(modelSet))
	for modelName := range modelSet {
		models = append(models, modelName)
	}
	sort.Strings(models)
	return models, nil
}

func supportsClawChat(endpointTypes []constant.EndpointType) bool {
	for _, endpointType := range endpointTypes {
		switch endpointType {
		case constant.EndpointTypeOpenAI,
			constant.EndpointTypeOpenAIResponse,
			constant.EndpointTypeOpenAIResponseCompact,
			constant.EndpointTypeAnthropic,
			constant.EndpointTypeGemini:
			return true
		}
	}
	return false
}

func pickClawDefaultModel(models []string) string {
	if len(models) == 0 {
		return ""
	}
	preferredKeywords := []string{"claude", "gpt", "gemini", "deepseek", "qwen"}
	for _, keyword := range preferredKeywords {
		for _, modelName := range models {
			if strings.Contains(strings.ToLower(modelName), keyword) {
				return modelName
			}
		}
	}
	return models[0]
}

func getClawProfileData(userID int) (clawProfileData, error) {
	user, err := model.GetUserById(userID, true)
	if err != nil {
		return clawProfileData{}, err
	}

	plan := "free"
	planName := "user.plan.free"
	if user.Role >= common.RoleAdminUser {
		plan = "team"
		planName = "user.plan.team"
	} else if subscriptions, err := model.GetAllActiveUserSubscriptions(userID); err == nil && len(subscriptions) > 0 {
		plan = "pro"
		planName = "user.plan.pro"
	}

	email := strings.TrimSpace(user.Email)
	if email == "" {
		email = strings.TrimSpace(user.Username)
	}
	if strings.TrimSpace(email) == "" {
		email = common.Interface2String(user.Id)
	}
	displayName := strings.TrimSpace(user.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(user.Username)
	}

	return clawProfileData{
		ID:           user.Id,
		Username:     user.Username,
		DisplayName:  displayName,
		Email:        email,
		Plan:         plan,
		PlanName:     planName,
		Quota:        user.Quota,
		UsedQuota:    user.UsedQuota,
		RequestCount: user.RequestCount,
	}, nil
}

func getClawModelConfig(c *gin.Context, userID int) (clawModelConfig, error) {
	models, err := listClawUserModels(c, userID)
	if err != nil {
		return clawModelConfig{}, err
	}
	return clawModelConfig{
		DefaultModel: pickClawDefaultModel(models),
		ModelList:    models,
		Mem0Enabled:  false,
	}, nil
}

func GetClawProfile(c *gin.Context) {
	profile, err := getClawProfileData(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Profile retrieved successfully",
		"data":    profile,
	})
}

func GetClawModels(c *gin.Context) {
	models, err := getClawModelConfig(c, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Models retrieved successfully",
		"data":    models,
	})
}

func GetClawBootstrap(c *gin.Context) {
	userID := c.GetInt("id")
	profile, err := getClawProfileData(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	models, err := getClawModelConfig(c, userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user, err := model.GetUserById(userID, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	accessToken, err := ensureUserAccessToken(user)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	apiBase := "/api/claw"
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Bootstrap retrieved successfully",
		"data": clawBootstrapData{
			User:           profile,
			Profile:        profile,
			ModelConfig:    models,
			Models:         models,
			APIBase:        apiBase,
			APIBaseURL:     apiBase,
			BackendURL:     apiBase,
			BackendURLAlt:  apiBase,
			ClawBase:       "/claw",
			FrontendUI:     "/console/claw",
			AccessToken:    accessToken,
			HasAccessToken: accessToken != "",
		},
	})
}

func ProxyClawBackend(c *gin.Context) {
	userID := c.GetInt("id")
	path := strings.TrimPrefix(c.Param("path"), "/")
	targetURL := "/api/v1"
	if path != "" {
		targetURL += "/" + path
	}
	backendURL := getClawBackendBaseURL() + targetURL
	if rawQuery := c.Request.URL.RawQuery; rawQuery != "" {
		backendURL += "?" + rawQuery
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, backendURL, c.Request.Body)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for key, values := range c.Request.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Del("Host")
	req.Header.Del("Content-Length")
	req.Header.Del("Authorization")
	req.Header.Del("Cookie")
	req.Header.Del("New-Api-User")
	req.Header.Set("X-User-Id", common.Interface2String(userID))
	if req.Header.Get("X-Forwarded-Proto") == "" {
		if c.Request.TLS != nil {
			req.Header.Set("X-Forwarded-Proto", "https")
		} else {
			req.Header.Set("X-Forwarded-Proto", "http")
		}
	}
	if req.Header.Get("X-Forwarded-Host") == "" && c.Request.Host != "" {
		req.Header.Set("X-Forwarded-Host", c.Request.Host)
	}
	if clientIP := strings.TrimSpace(c.ClientIP()); clientIP != "" {
		req.Header.Set("X-Forwarded-For", clientIP)
	}

	client := service.GetHttpClient()
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
}
