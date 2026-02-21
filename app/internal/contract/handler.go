package contract

import (
	"context"
	"os"
	"path/filepath"
	"strconv"

	"contract_review/app/internal/global"
	"contract_review/app/internal/middleware"
	"contract_review/app/pkg/response"

	"github.com/cloudwego/hertz/pkg/app"
)

type ContractHandler struct {
	contractService *ContractService
}

func NewContractHandler(contractService *ContractService) *ContractHandler {
	return &ContractHandler{contractService: contractService}
}

// ============ Contract 相关接口 ============

// UploadContractFile 上传合同文件
// @Summary 上传合同文件
// @Description 上传合同文件并自动解析甲方、乙方、金额等信息
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "合同文件(PDF/DOCX)"
// @Success 200 {object} UploadContractResponse
// @Router /api/contract/upload [post]
func (ch *ContractHandler) UploadContractFile(ctx context.Context, c *app.RequestContext) {
	// 1. 验证用户登录状态
	account := middleware.GetCurrentUserID(c)
	if account == "" {
		global.Log.Error("未登录不能上传")
		c.JSON(401, response.Unauthorized())
		return
	}

	// 2. 获取上传的文件
	file, err := c.FormFile("file")
	if err != nil {
		global.Log.Error("获取文件失败，文件不能为空")
		c.JSON(400, response.FailWithMsg("文件不能为空"))
		return
	}

	// 3. 验证文件类型
	ext := filepath.Ext(file.Filename)
	if ext != ".pdf" && ext != ".docx" && ext != ".txt" {
		global.Log.Error("不支持的文件格式")
		c.JSON(400, response.FailWithMsg("不支持的文件格式，仅支持 PDF、DOCX、TXT"))
		return
	}

	// 4. 确保上传目录存在
	uploadDir := "uploads"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		global.Log.Error("创建上传目录失败")
		c.JSON(500, response.ServerError())
		return
	}

	// 5. 保存文件到本地
	savePath := filepath.Join(uploadDir, file.Filename)
	if err := c.SaveUploadedFile(file, savePath); err != nil {
		global.Log.Error("保存文件失败")
		c.JSON(500, response.ServerError())
		return
	}

	// 6. 调用服务层处理文件（提取文本、LLM解析、保存数据库）
	contractDetail, err := ch.contractService.FileLoad(ctx, account, savePath, file.Filename)
	if err != nil {
		global.Log.Error("处理合同文件失败: " + err.Error())
		c.JSON(500, response.FailWithMsg("处理合同文件失败："+err.Error()))
		return
	}

	// 7. 构建响应
	resp := UploadContractResponse{
		FileID:         contractDetail.ID,
		Title:          contractDetail.Title,
		FilePathURL:    contractDetail.FilePath,
		FileType:       contractDetail.FileType,
		ContractTypeID: contractDetail.TypeID,
		PartyA:         contractDetail.PartyA,
		PartyB:         contractDetail.PartyB,
		Amount:         contractDetail.Amount,
	}

	c.JSON(200, response.OkWithData(resp))
}

// ListContracts 获取用户合同列表
// @Summary 获取用户合同列表
// @Description 分页获取当前用户的合同列表
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} ContractListResponse
// @Router /api/contract/list [get]
func (ch *ContractHandler) ListContracts(ctx context.Context, c *app.RequestContext) {
	account := middleware.GetCurrentUserID(c)
	if account == "" {
		c.JSON(401, response.Unauthorized())
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	contracts, total, err := ch.contractService.ListContractsByAccountWithType(ctx, account, page, pageSize)
	if err != nil {
		c.JSON(500, response.ServerError())
		return
	}

	c.JSON(200, response.OkWithData(map[string]interface{}{
		"list":      contracts,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}))
}

// UpdateContractType 更新合同的类型
// @Summary 更新合同类型
// @Description 更新指定合同的类型ID
// @Accept json
// @Produce json
// @Param id path int true "合同ID"
// @Param body body UpdateContractTypeRequest true "请求体"
// @Success 200 {object} response.Result
// @Router /api/contract/:id/type [put]
func (ch *ContractHandler) UpdateContractType(ctx context.Context, c *app.RequestContext) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的合同ID"))
		return
	}

	var req UpdateContractTypeRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, response.FailWithMsg("请求参数错误"))
		return
	}

	if err := ch.contractService.UpdateContractTypeID(ctx, id, req.TypeID); err != nil {
		c.JSON(400, response.FailWithMsg(err.Error()))
		return
	}

	c.JSON(200, response.Ok())
}

// DeleteContract 删除合同
// @Summary 删除合同
// @Description 根据合同ID删除合同
// @Produce json
// @Param id path int true "合同ID"
// @Success 200 {object} response.Result
// @Router /api/contract/:id [delete]
func (ch *ContractHandler) DeleteContract(ctx context.Context, c *app.RequestContext) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的合同ID"))
		return
	}

	if err := ch.contractService.DeleteContract(ctx, id); err != nil {
		c.JSON(500, response.FailWithMsg("删除合同失败"))
		return
	}

	c.JSON(200, response.Ok())
}

// DownloadContract 下载合同文件
// @Summary 下载合同文件
// @Description 根据合同ID下载合同文件
// @Produce application/octet-stream
// @Param id path int true "合同ID"
// @Success 200 {file} file
// @Router /api/contract/:id/download [get]
func (ch *ContractHandler) DownloadContract(ctx context.Context, c *app.RequestContext) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的合同ID"))
		return
	}

	filePath, filename, err := ch.contractService.GetContractFilePath(ctx, id)
	if err != nil {
		c.JSON(404, response.FailWithMsg("文件不存在"))
		return
	}

	// 设置响应头
	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Header("Content-Type", "application/octet-stream")
	c.File(filePath)
}

// ============ ContractType 相关接口 ============

// CreateContractType 创建合同类型
// @Summary 创建合同类型
// @Description 创建新的合同类型
// @Accept json
// @Produce json
// @Param body body CreateContractTypeRequest true "请求体"
// @Success 200 {object} ContractTypeResponse
// @Router /api/contract-type [post]
func (ch *ContractHandler) CreateContractType(ctx context.Context, c *app.RequestContext) {
	var req CreateContractTypeRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, response.FailWithMsg("请求参数错误"))
		return
	}

	if req.Name == "" {
		c.JSON(400, response.FailWithMsg("类型名称不能为空"))
		return
	}

	contractType, err := ch.contractService.CreateContractType(ctx, req.Name)
	if err != nil {
		c.JSON(400, response.FailWithMsg(err.Error()))
		return
	}

	c.JSON(200, response.OkWithData(ContractTypeResponse{
		ID:        contractType.ID,
		Name:      contractType.Name,
		CreatedAt: contractType.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt: contractType.UpdatedAt.Format("2006-01-02 15:04:05"),
	}))
}

// GetContractType 获取合同类型详情
// @Summary 获取合同类型详情
// @Description 根据ID获取合同类型详情
// @Produce json
// @Param id path int true "类型ID"
// @Success 200 {object} ContractTypeResponse
// @Router /api/contract-type/:id [get]
func (ch *ContractHandler) GetContractType(ctx context.Context, c *app.RequestContext) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的类型ID"))
		return
	}

	contractType, err := ch.contractService.GetContractTypeByID(ctx, id)
	if err != nil {
		c.JSON(404, response.FailWithMsg("合同类型不存在"))
		return
	}

	// 获取使用数量
	count, _ := ch.contractService.GetContractTypeUsageCount(ctx, id)

	c.JSON(200, response.OkWithData(ContractTypeDetailResponse{
		ID:            contractType.ID,
		Name:          contractType.Name,
		CreatedAt:     contractType.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:     contractType.UpdatedAt.Format("2006-01-02 15:04:05"),
		ContractCount: count,
	}))
}

// ListContractTypes 获取合同类型列表
// @Summary 获取合同类型列表
// @Description 获取所有合同类型列表
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} ContractTypeListResponse
// @Router /api/contract-type/list [get]
func (ch *ContractHandler) ListContractTypes(ctx context.Context, c *app.RequestContext) {
	pageStr := c.DefaultQuery("page", "")
	pageSizeStr := c.DefaultQuery("page_size", "")

	// 如果没有分页参数，返回所有类型
	if pageStr == "" && pageSizeStr == "" {
		types, err := ch.contractService.ListContractTypes(ctx)
		if err != nil {
			c.JSON(500, response.ServerError())
			return
		}

		var list []ContractTypeResponse
		for _, t := range types {
			list = append(list, ContractTypeResponse{
				ID:        t.ID,
				Name:      t.Name,
				CreatedAt: t.CreatedAt.Format("2006-01-02 15:04:05"),
				UpdatedAt: t.UpdatedAt.Format("2006-01-02 15:04:05"),
			})
		}

		c.JSON(200, response.OkWithData(map[string]interface{}{
			"list":  list,
			"total": len(list),
		}))
		return
	}

	// 分页查询
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	types, total, err := ch.contractService.ListContractTypesPaginated(ctx, page, pageSize)
	if err != nil {
		c.JSON(500, response.ServerError())
		return
	}

	var list []ContractTypeResponse
	for _, t := range types {
		list = append(list, ContractTypeResponse{
			ID:        t.ID,
			Name:      t.Name,
			CreatedAt: t.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt: t.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	c.JSON(200, response.OkWithData(map[string]interface{}{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}))
}

// UpdateContractTypeName 更新合同类型名称
// @Summary 更新合同类型
// @Description 更新合同类型名称
// @Accept json
// @Produce json
// @Param id path int true "类型ID"
// @Param body body UpdateContractTypeNameRequest true "请求体"
// @Success 200 {object} response.Result
// @Router /api/contract-type/:id [put]
func (ch *ContractHandler) UpdateContractTypeName(ctx context.Context, c *app.RequestContext) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的类型ID"))
		return
	}

	var req UpdateContractTypeNameRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, response.FailWithMsg("请求参数错误"))
		return
	}

	if req.Name == "" {
		c.JSON(400, response.FailWithMsg("类型名称不能为空"))
		return
	}

	if err := ch.contractService.UpdateContractType(ctx, id, req.Name); err != nil {
		c.JSON(400, response.FailWithMsg(err.Error()))
		return
	}

	c.JSON(200, response.Ok())
}

// DeleteContractType 删除合同类型
// @Summary 删除合同类型
// @Description 删除合同类型（如果该类型下有合同则无法删除）
// @Produce json
// @Param id path int true "类型ID"
// @Success 200 {object} response.Result
// @Router /api/contract-type/:id [delete]
func (ch *ContractHandler) DeleteContractType(ctx context.Context, c *app.RequestContext) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的类型ID"))
		return
	}

	if err := ch.contractService.DeleteContractType(ctx, id); err != nil {
		c.JSON(400, response.FailWithMsg(err.Error()))
		return
	}

	c.JSON(200, response.Ok())
}
