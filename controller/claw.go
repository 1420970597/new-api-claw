package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

type clawModelVendor struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
}

type clawModelEndpointDefinition struct {
	Path   string `json:"path"`
	Method string `json:"method"`
}

type clawModelMetadata struct {
	ModelName              string                                 `json:"model_name"`
	Description            string                                 `json:"description,omitempty"`
	Icon                   string                                 `json:"icon,omitempty"`
	Tags                   []string                               `json:"tags,omitempty"`
	VendorID               int                                    `json:"vendor_id,omitempty"`
	Vendor                 *clawModelVendor                       `json:"vendor,omitempty"`
	SupportedEndpointTypes []constant.EndpointType                `json:"supported_endpoint_types,omitempty"`
	EndpointDefinitions    map[string]clawModelEndpointDefinition `json:"endpoint_definitions,omitempty"`
}

type clawModelConfig struct {
	DefaultModel string              `json:"default_model"`
	ModelList    []string            `json:"model_list"`
	Models       []clawModelMetadata `json:"models"`
	Mem0Enabled  bool                `json:"mem0_enabled"`
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

type clawUpstreamModelsResponse struct {
	Success bool            `json:"success"`
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
}

type clawUpstreamModelItem struct {
	ID string `json:"id"`
}

type clawUpstreamModelsPayload struct {
	ModelList []string `json:"model_list"`
}

var clawAllowedProxyResources = map[string]struct{}{
	"attachments":         {},
	"callback":            {},
	"claude-md":           {},
	"env-vars":            {},
	"health":              {},
	"mcp-installs":        {},
	"mcp-servers":         {},
	"memories":            {},
	"messages":            {},
	"models":              {},
	"plugin-installs":     {},
	"plugins":             {},
	"projects":            {},
	"runs":                {},
	"scheduled-tasks":     {},
	"search":              {},
	"sessions":            {},
	"skill-installs":      {},
	"skills":              {},
	"slash-commands":      {},
	"subagents":           {},
	"tasks":               {},
	"tool-executions":     {},
	"usage":               {},
	"user-input-requests": {},
}

var clawForwardRequestHeaders = []string{
	"Accept",
	"Accept-Encoding",
	"Accept-Language",
	"Cache-Control",
	"Content-Type",
	"If-Modified-Since",
	"If-None-Match",
	"Range",
	"User-Agent",
	"X-Request-Id",
}

var clawForwardResponseHeaders = []string{
	"Cache-Control",
	"Content-Disposition",
	"Content-Encoding",
	"Content-Length",
	"Content-Type",
	"ETag",
	"Last-Modified",
	"X-Accel-Buffering",
	"X-Request-Id",
}

func normalizeClawProxyPath(path string) string {
	return strings.Trim(strings.TrimSpace(path), "/")
}

func isAllowedClawProxyPath(path string) bool {
	normalizedPath := normalizeClawProxyPath(path)
	if normalizedPath == "" {
		return false
	}

	resource := normalizedPath
	if idx := strings.IndexByte(normalizedPath, '/'); idx >= 0 {
		resource = normalizedPath[:idx]
	}

	_, allowed := clawAllowedProxyResources[resource]
	return allowed
}

func copyAllowedHeaders(src http.Header, dst http.Header, allowedKeys []string) {
	for _, key := range allowedKeys {
		values := src.Values(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func getClawBackendBaseURL() string {
	baseURL := strings.TrimSpace(os.Getenv("POCO_CLAW_BACKEND_URL"))
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8000"
	}
	return strings.TrimSuffix(baseURL, "/")
}

func parseClawUpstreamModelNames(data json.RawMessage) []string {
	if len(data) == 0 || strings.TrimSpace(string(data)) == "" || strings.TrimSpace(string(data)) == "null" {
		return nil
	}

	resultSet := make(map[string]struct{})
	appendName := func(name string) {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return
		}
		resultSet[trimmed] = struct{}{}
	}

	var payload clawUpstreamModelsPayload
	if err := common.Unmarshal(data, &payload); err == nil {
		for _, name := range payload.ModelList {
			appendName(name)
		}
	}

	var items []clawUpstreamModelItem
	if err := common.Unmarshal(data, &items); err == nil {
		for _, item := range items {
			appendName(item.ID)
		}
	}

	var names []string
	if err := common.Unmarshal(data, &names); err == nil {
		for _, name := range names {
			appendName(name)
		}
	}

	models := make([]string, 0, len(resultSet))
	for name := range resultSet {
		models = append(models, name)
	}
	return models
}

func getClawUserGroup(c *gin.Context, userID int) (string, error) {
	group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	if strings.TrimSpace(group) != "" {
		return group, nil
	}
	return model.GetUserGroup(userID, false)
}

func getClawUpstreamModels(c *gin.Context, userID int) ([]string, error) {
	group, err := getClawUserGroup(c, userID)
	if err != nil {
		return nil, err
	}

	groups := make([]string, 0, 4)
	if group == "auto" {
		userCache, err := model.GetUserCache(userID)
		if err != nil {
			return nil, err
		}
		for _, autoGroup := range service.GetUserAutoGroup(userCache.Group) {
			autoGroup = strings.TrimSpace(autoGroup)
			if autoGroup == "" {
				continue
			}
			groups = append(groups, autoGroup)
		}
	} else if strings.TrimSpace(group) != "" {
		groups = append(groups, strings.TrimSpace(group))
	}

	requestURLs := make([]string, 0, len(groups)+1)
	for _, g := range groups {
		requestURLs = append(requestURLs, fmt.Sprintf("%s/api/v1/models?group=%s", getClawBackendBaseURL(), url.QueryEscape(g)))
	}
	requestURLs = append(requestURLs, fmt.Sprintf("%s/api/v1/models", getClawBackendBaseURL()))

	client := service.GetHttpClient()
	if client == nil {
		client = http.DefaultClient
	}

	modelSet := make(map[string]struct{})
	successCount := 0
	for _, backendURL := range requestURLs {
		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, backendURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-User-Id", common.Interface2String(userID))

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			resp.Body.Close()
			continue
		}

		var upstream clawUpstreamModelsResponse
		if err = common.DecodeJson(resp.Body, &upstream); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()
		successCount++

		for _, name := range parseClawUpstreamModelNames(upstream.Data) {
			if endpointTypes := model.GetModelSupportEndpointTypes(name); len(endpointTypes) > 0 && !supportsClawChat(endpointTypes) {
				continue
			}
			modelSet[name] = struct{}{}
		}
	}

	if successCount == 0 {
		return nil, fmt.Errorf("failed to fetch claw upstream models")
	}
	if len(modelSet) == 0 {
		return nil, fmt.Errorf("claw upstream models are empty")
	}

	models := make([]string, 0, len(modelSet))
	for name := range modelSet {
		models = append(models, name)
	}
	sort.Strings(models)
	return models, nil
}

func listClawUserModels(c *gin.Context, userID int) ([]string, error) {
	upstreamModels, upstreamErr := getClawUpstreamModels(c, userID)
	if upstreamErr == nil {
		return upstreamModels, nil
	}
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

func parseClawModelTags(rawTags string) []string {
	normalized := strings.NewReplacer(",", ",", "，", ",", ";", ",", "；", ",", "|", ",", "\n", ",", "\t", ",").Replace(rawTags)
	parts := strings.Split(normalized, ",")
	if len(parts) == 0 {
		return nil
	}
	tags := make([]string, 0, len(parts))
	tagSet := make(map[string]struct{})
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag == "" {
			continue
		}
		if _, exists := tagSet[tag]; exists {
			continue
		}
		tagSet[tag] = struct{}{}
		tags = append(tags, tag)
	}
	if len(tags) == 0 {
		return nil
	}
	return tags
}

func buildClawModelMetadata(modelList []string) []clawModelMetadata {
	if len(modelList) == 0 {
		return make([]clawModelMetadata, 0)
	}

	pricingList := model.GetPricing()
	pricingByModel := make(map[string]model.Pricing, len(pricingList))
	for _, pricing := range pricingList {
		pricingByModel[pricing.ModelName] = pricing
	}

	vendorMap := make(map[int]clawModelVendor)
	for _, vendor := range model.GetVendors() {
		vendorMap[vendor.ID] = clawModelVendor{
			ID:          vendor.ID,
			Name:        vendor.Name,
			Description: vendor.Description,
			Icon:        vendor.Icon,
		}
	}

	supportedEndpointMap := model.GetSupportedEndpointMap()
	metadataList := make([]clawModelMetadata, 0, len(modelList))
	for _, modelName := range modelList {
		metadata := clawModelMetadata{
			ModelName: modelName,
		}

		if pricing, ok := pricingByModel[modelName]; ok {
			metadata.Description = strings.TrimSpace(pricing.Description)
			metadata.Icon = strings.TrimSpace(pricing.Icon)
			metadata.Tags = parseClawModelTags(pricing.Tags)
			metadata.VendorID = pricing.VendorID
			if len(pricing.SupportedEndpointTypes) > 0 {
				metadata.SupportedEndpointTypes = append(metadata.SupportedEndpointTypes, pricing.SupportedEndpointTypes...)
			}
		}

		if len(metadata.SupportedEndpointTypes) == 0 {
			endpointTypes := model.GetModelSupportEndpointTypes(modelName)
			if len(endpointTypes) > 0 {
				metadata.SupportedEndpointTypes = append(metadata.SupportedEndpointTypes, endpointTypes...)
			}
		}

		if metadata.VendorID > 0 {
			if vendor, ok := vendorMap[metadata.VendorID]; ok {
				vendorCopy := vendor
				metadata.Vendor = &vendorCopy
			}
		}

		if len(metadata.SupportedEndpointTypes) > 0 {
			endpointDefinitions := make(map[string]clawModelEndpointDefinition)
			for _, endpointType := range metadata.SupportedEndpointTypes {
				endpointKey := string(endpointType)
				endpointInfo, ok := supportedEndpointMap[endpointKey]
				if !ok {
					if defaultInfo, exists := common.GetDefaultEndpointInfo(endpointType); exists {
						endpointInfo = defaultInfo
						ok = true
					}
				}
				if !ok {
					continue
				}
				endpointDefinitions[endpointKey] = clawModelEndpointDefinition{
					Path:   endpointInfo.Path,
					Method: endpointInfo.Method,
				}
			}
			if len(endpointDefinitions) > 0 {
				metadata.EndpointDefinitions = endpointDefinitions
			}
		}

		metadataList = append(metadataList, metadata)
	}

	return metadataList
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
		Models:       buildClawModelMetadata(models),
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
	path := normalizeClawProxyPath(c.Param("path"))
	if subpath := normalizeClawProxyPath(c.Param("subpath")); subpath != "" {
		if path != "" {
			path += "/" + subpath
		} else {
			path = subpath
		}
	}

	if !isAllowedClawProxyPath(path) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "claw proxy path is not allowed",
		})
		return
	}

	targetURL := "/api/v1/" + path
	backendURL := getClawBackendBaseURL() + targetURL
	if rawQuery := c.Request.URL.RawQuery; rawQuery != "" {
		backendURL += "?" + rawQuery
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, backendURL, c.Request.Body)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	copyAllowedHeaders(c.Request.Header, req.Header, clawForwardRequestHeaders)
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

	copyAllowedHeaders(resp.Header, c.Writer.Header(), clawForwardResponseHeaders)
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
}
