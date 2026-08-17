package contract

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"contract_review/app/internal/global"
	"contract_review/app/internal/middleware"
	"contract_review/app/pkg/response"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/google/uuid"
	"gorm.io/gorm"
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
	// 1. 验证用户登录状态（ResourceScope 已由 ResolveScope 中间件解析）
	scope, ok := middleware.GetScope(c)
	if !ok {
		global.Log.Error("未登录不能上传")
		c.JSON(401, response.Unauthorized())
		return
	}
	account := scope.Account

	// 2. 获取上传的文件
	file, err := c.FormFile("file")
	if err != nil {
		global.Log.Error("获取文件失败，文件不能为空")
		c.JSON(400, response.FailWithMsg("文件不能为空"))
		return
	}

	// 3. 验证文件类型和内容
	originalName := sanitizeFilename(file.Filename)
	ext := strings.ToLower(filepath.Ext(originalName))
	if ext != ".pdf" && ext != ".docx" && ext != ".txt" {
		global.Log.Error("不支持的文件格式")
		c.JSON(400, response.FailWithMsg("不支持的文件格式，仅支持 PDF、DOCX、TXT"))
		return
	}
	if err := validateUploadedFile(file, ext); err != nil {
		c.JSON(400, response.FailWithMsg(err.Error()))
		return
	}

	// 4. 确保上传目录存在
	uploadDir := UploadDir()
	storageDir := filepath.Join(uploadDir, accountStorageDir(account))
	if err := os.MkdirAll(storageDir, 0700); err != nil {
		global.Log.Error("创建上传目录失败")
		c.JSON(500, response.ServerError())
		return
	}

	// 5. 使用不可预测 object key 保存文件，原文件名只作为展示元数据。
	savePath := filepath.Join(storageDir, uuid.NewString()+ext)
	if err := c.SaveUploadedFile(file, savePath); err != nil {
		global.Log.Error("保存文件失败")
		c.JSON(500, response.ServerError())
		return
	}
	_ = os.Chmod(savePath, 0600)

	// 6. 调用服务层处理文件（提取文本、LLM解析、保存数据库）
	contractDetail, err := ch.contractService.FileLoad(ctx, account, savePath, originalName)
	if err != nil {
		_ = os.Remove(savePath)
		global.Log.Error("处理合同文件失败: " + err.Error())
		c.JSON(500, response.FailWithMsg("处理合同文件失败："+err.Error()))
		return
	}

	// 7. 构建响应
	resp := UploadContractResponse{
		FileID:         contractDetail.ID,
		Title:          contractDetail.Title,
		FilePathURL:    DownloadURL(contractDetail.ID),
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
	scope, ok := middleware.GetScope(c)
	if !ok {
		c.JSON(401, response.Unauthorized())
		return
	}
	account := scope.Account

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
	scope, ok := middleware.GetScope(c)
	if !ok {
		c.JSON(401, response.Unauthorized())
		return
	}
	account := scope.Account
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

	if err := ch.contractService.UpdateContractTypeID(ctx, account, id, req.TypeID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, response.NotFound())
			return
		}
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
	scope, ok := middleware.GetScope(c)
	if !ok {
		c.JSON(401, response.Unauthorized())
		return
	}
	account := scope.Account
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的合同ID"))
		return
	}

	if err := ch.contractService.DeleteContract(ctx, account, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, response.NotFound())
			return
		}
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
	scope, ok := middleware.GetScope(c)
	if !ok {
		c.JSON(401, response.Unauthorized())
		return
	}
	account := scope.Account
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的合同ID"))
		return
	}

	filePath, filename, fileType, err := ch.contractService.GetContractFilePath(ctx, account, id)
	if err != nil {
		c.JSON(404, response.FailWithMsg("文件不存在"))
		return
	}

	// 设置安全的下载响应头，避免原始名称中的 CRLF 注入。
	filename = sanitizeFilename(filename)
	encodedFilename := url.PathEscape(filename)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s", filename, encodedFilename))
	contentType := mime.TypeByExtension("." + strings.ToLower(strings.TrimPrefix(fileType, ".")))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Content-Type", contentType)
	c.File(filePath)
}

const maxUploadSize int64 = 50 << 20

func sanitizeFilename(filename string) string {
	filename = filepath.Base(strings.TrimSpace(filename))
	filename = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '"' || r == '\\' {
			return -1
		}
		return r
	}, filename)
	if filename == "" || filename == "." || filename == ".." {
		return "contract"
	}
	return filename
}

func accountStorageDir(account string) string {
	sum := sha256.Sum256([]byte(account))
	return hex.EncodeToString(sum[:8])
}

func validateUploadedFile(file *multipart.FileHeader, ext string) error {
	if file == nil || file.Size <= 0 {
		return errors.New("文件不能为空")
	}
	if file.Size > maxUploadSize {
		return fmt.Errorf("文件不能超过 %d MB", maxUploadSize/(1<<20))
	}

	reader, err := file.Open()
	if err != nil {
		return errors.New("无法读取上传文件")
	}
	defer reader.Close()

	header := make([]byte, 512)
	n, readErr := io.ReadFull(reader, header)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return errors.New("无法读取上传文件")
	}
	header = header[:n]
	if len(header) == 0 {
		return errors.New("文件不能为空")
	}

	switch ext {
	case ".pdf":
		if !bytes.HasPrefix(header, []byte("%PDF-")) {
			return errors.New("文件内容不是有效的 PDF")
		}
	case ".docx":
		if len(header) < 4 || header[0] != 'P' || header[1] != 'K' {
			return errors.New("文件内容不是有效的 DOCX")
		}
		if err := validateDOCXArchive(file); err != nil {
			return err
		}
	case ".txt":
		if bytes.IndexByte(header, 0) >= 0 {
			return errors.New("TXT 文件包含二进制内容")
		}
	}

	_ = http.DetectContentType(header)
	return nil
}

func validateDOCXArchive(file *multipart.FileHeader) error {
	reader, err := file.Open()
	if err != nil {
		return errors.New("无法读取 DOCX 文件")
	}
	defer reader.Close()

	readerAt, ok := reader.(io.ReaderAt)
	if !ok {
		return errors.New("无法校验 DOCX 文件结构")
	}
	return validateDOCXReader(readerAt, file.Size)
}

func validateDOCXReader(reader io.ReaderAt, size int64) error {
	archive, err := zip.NewReader(reader, size)
	if err != nil {
		return errors.New("文件内容不是有效的 DOCX")
	}
	if len(archive.File) > 5000 {
		return errors.New("DOCX 文件包含过多条目")
	}

	const (
		maxEntrySize        = uint64(100 << 20)
		maxUncompressedSize = uint64(200 << 20)
	)
	var totalSize uint64
	hasContentTypes := false
	hasDocument := false

	for _, entry := range archive.File {
		name := strings.ReplaceAll(entry.Name, "\\", "/")
		cleanName := path.Clean(name)
		if cleanName == ".." || strings.HasPrefix(cleanName, "../") || path.IsAbs(cleanName) {
			return errors.New("DOCX 文件包含非法路径")
		}
		if entry.UncompressedSize64 > maxEntrySize {
			return errors.New("DOCX 文件内部条目过大")
		}
		if totalSize > maxUncompressedSize-entry.UncompressedSize64 {
			return errors.New("DOCX 文件解压后内容过大")
		}
		totalSize += entry.UncompressedSize64

		switch cleanName {
		case "[Content_Types].xml":
			hasContentTypes = true
		case "word/document.xml":
			hasDocument = true
		}
	}

	if !hasContentTypes || !hasDocument {
		return errors.New("ZIP 文件不是有效的 DOCX 文档")
	}
	return nil
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

	name := normalizeContractTypeName(req.Name, req.ContractTypeName)
	if name == "" {
		c.JSON(400, response.FailWithMsg("类型名称不能为空"))
		return
	}

	creatorScope, ok := middleware.GetScope(c)
	if !ok {
		c.JSON(401, response.Unauthorized())
		return
	}
	creator := creatorScope.Account
	contractType, err := ch.contractService.CreateContractType(ctx, name, req.TemplateContent, creator)
	if err != nil {
		c.JSON(400, response.FailWithMsg(err.Error()))
		return
	}

	c.JSON(200, response.OkWithData(ch.contractTypeToResponse(contractType)))
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

	resp := ch.contractTypeToDetailResponse(contractType)
	resp.ContractCount = count
	c.JSON(200, response.OkWithData(resp))
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
	pageSizeStr := c.DefaultQuery("page_size", c.DefaultQuery("pageSize", ""))
	name := c.DefaultQuery("contractTypeName", c.DefaultQuery("name", ""))
	creator := c.DefaultQuery("creator", "")
	startDate := c.DefaultQuery("startDate", "")
	endDate := c.DefaultQuery("endDate", "")

	// 如果没有分页参数，返回所有类型
	if pageStr == "" && pageSizeStr == "" && name == "" && creator == "" && startDate == "" && endDate == "" {
		types, err := ch.contractService.ListContractTypes(ctx)
		if err != nil {
			c.JSON(500, response.ServerError())
			return
		}

		var list []ContractTypeResponse
		for _, t := range types {
			list = append(list, ch.contractTypeToResponse(&t))
		}

		c.JSON(200, response.OkWithData(map[string]interface{}{
			"list":  list,
			"total": len(list),
		}))
		return
	}

	// 分页查询
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", c.DefaultQuery("pageSize", "20")))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	types, total, err := ch.contractService.ListContractTypesFiltered(ctx, name, creator, startDate, endDate, page, pageSize)
	if err != nil {
		c.JSON(500, response.ServerError())
		return
	}

	var list []ContractTypeResponse
	for _, t := range types {
		list = append(list, ch.contractTypeToResponse(&t))
	}

	c.JSON(200, response.OkWithData(map[string]interface{}{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"pageSize":  pageSize,
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

	name := normalizeContractTypeName(req.Name, req.ContractTypeName)
	if name == "" {
		c.JSON(400, response.FailWithMsg("类型名称不能为空"))
		return
	}

	if err := ch.contractService.UpdateContractType(ctx, id, name, req.TemplateContent); err != nil {
		c.JSON(400, response.FailWithMsg(err.Error()))
		return
	}

	c.JSON(200, response.Ok())
}

// BatchDeleteContractType 批量删除合同类型
func (ch *ContractHandler) BatchDeleteContractType(ctx context.Context, c *app.RequestContext) {
	var req struct {
		IDs []uint64 `json:"ids"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, response.FailWithMsg("请求参数错误"))
		return
	}
	if len(req.IDs) == 0 {
		c.JSON(400, response.FailWithMsg("请选择要删除的合同类型"))
		return
	}
	if err := ch.contractService.DeleteContractTypes(ctx, req.IDs); err != nil {
		c.JSON(400, response.FailWithMsg(err.Error()))
		return
	}
	c.JSON(200, response.Ok())
}

// ListContractTypeCreators 获取合同类型创建人列表
func (ch *ContractHandler) ListContractTypeCreators(ctx context.Context, c *app.RequestContext) {
	creators, err := ch.contractService.ListContractTypeCreators(ctx)
	if err != nil {
		c.JSON(500, response.ServerError())
		return
	}
	list := make([]ContractTypeCreatorResponse, 0, len(creators))
	for _, creator := range creators {
		list = append(list, ContractTypeCreatorResponse{
			ID:   creator,
			Name: creator,
		})
	}
	c.JSON(200, response.OkWithData(list))
}

func normalizeContractTypeName(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (ch *ContractHandler) contractTypeToResponse(contractType *ContractType) ContractTypeResponse {
	updatedAt := contractType.UpdatedAt.Format("2006-01-02 15:04:05")
	return ContractTypeResponse{
		ID:               contractType.ID,
		Name:             contractType.Name,
		ContractTypeName: contractType.Name,
		TemplateContent:  contractType.TemplateContent,
		Creator:          contractType.Creator,
		CreatedAt:        contractType.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:        updatedAt,
		UpdateDate:       updatedAt,
	}
}

func (ch *ContractHandler) contractTypeToDetailResponse(contractType *ContractType) ContractTypeDetailResponse {
	updatedAt := contractType.UpdatedAt.Format("2006-01-02 15:04:05")
	return ContractTypeDetailResponse{
		ID:               contractType.ID,
		Name:             contractType.Name,
		ContractTypeName: contractType.Name,
		TemplateContent:  contractType.TemplateContent,
		Creator:          contractType.Creator,
		CreatedAt:        contractType.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:        updatedAt,
		UpdateDate:       updatedAt,
	}
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
