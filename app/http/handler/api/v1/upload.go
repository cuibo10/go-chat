package v1

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"go-chat/app/http/request"
	"go-chat/app/http/response"
	"go-chat/app/pkg/auth"
	"go-chat/app/pkg/encrypt"
	"go-chat/app/pkg/filesystem"
	"go-chat/app/pkg/strutil"
	"go-chat/app/service"
	"go-chat/config"
	"strings"
	"time"
)

type Upload struct {
	config     *config.Config
	filesystem *filesystem.Filesystem
	service    *service.SplitUploadService
}

func NewUploadHandler(
	config *config.Config,
	filesystem *filesystem.Filesystem,
	service *service.SplitUploadService,
) *Upload {
	return &Upload{
		config:     config,
		filesystem: filesystem,
		service:    service,
	}
}

// 文件流上传
func (u *Upload) Stream(ctx *gin.Context) {
	params := &request.UploadFileStreamRequest{}
	if err := ctx.ShouldBind(params); err != nil {
		response.InvalidParams(ctx, err)
		return
	}

	params.Stream = strings.Replace(params.Stream, "data:image/png;base64,", "", 1)
	params.Stream = strings.Replace(params.Stream, " ", "+", 1)

	stream, _ := encrypt.Base64Decode(params.Stream)

	object := fmt.Sprintf("public/media/image/avatar/%s/%s", time.Now().Format("20060102"), strutil.GenImageName("png", 200, 200))

	err := u.filesystem.Default.Write(stream, object)
	if err != nil {
		response.BusinessError(ctx, "文件上传失败")
		return
	}

	response.Success(ctx, gin.H{
		"avatar": u.filesystem.Default.PublicUrl(object),
	})
}

// 批量上传初始化
func (u *Upload) InitiateMultipart(ctx *gin.Context) {
	params := &request.UploadInitiateMultipartRequest{}
	if err := ctx.ShouldBind(params); err != nil {
		response.InvalidParams(ctx, err)
		return
	}

	info, err := u.service.InitiateMultipartUpload(ctx.Request.Context(), &service.MultipartInitiateOpts{
		Name:   params.FileName,
		Size:   params.FileSize,
		UserId: auth.GetAuthUserID(ctx),
	})
	if err != nil {
		response.BusinessError(ctx, err)
		return
	}

	response.Success(ctx, &gin.H{
		"file_type":     info.Type,
		"user_id":       info.UserId,
		"original_name": info.OriginalName,
		"hash_name":     info.UploadId,
		"file_ext":      info.FileExt,
		"file_size":     info.FileSize,
		"split_num":     info.SplitNum,
		"split_index":   info.SplitIndex,
		"split_size":    2 << 20,
	})
}

// 批量分片上传
func (u *Upload) MultipartUpload(ctx *gin.Context) {
	params := &request.UploadMultipartRequest{}
	if err := ctx.ShouldBind(params); err != nil {
		response.InvalidParams(ctx, err)
		return
	}

	file, err := ctx.FormFile("file")
	if err != nil {
		response.InvalidParams(ctx, "文件上传失败！")
		return
	}

	err = u.service.MultipartUpload(ctx.Request.Context(), &service.MultipartUploadOpts{
		UserId:     auth.GetAuthUserID(ctx),
		UploadId:   params.UploadId,
		Name:       params.Name,
		Ext:        params.Ext,
		SplitIndex: params.SplitIndex,
		SplitNum:   params.SplitNum,
		File:       file,
	})
	if err != nil {
		response.BusinessError(ctx, err)
	}

	if params.SplitIndex != params.SplitNum-1 {
		response.Success(ctx, gin.H{"is_file_merge": false})
	} else {
		response.Success(ctx, gin.H{"is_file_merge": true, "hash": params.UploadId})
	}
}
