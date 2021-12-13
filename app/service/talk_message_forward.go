package service

import (
	"context"
	"errors"
	"go-chat/app/entity"
	"go-chat/app/model"
	"go-chat/app/pkg/jsonutil"
	"go-chat/app/pkg/slice"
	"go-chat/app/pkg/strutil"
	"gorm.io/gorm"
	"strings"
)

type ForwardParams struct {
	UserId     int   `json:"user_id"`
	ReceiverId int   `json:"receiver_id"`
	TalkType   int   `json:"talk_type"`
	RecordsIds []int `json:"records_ids"`
	UserIds    []int `json:"user_ids"`
	GroupIds   []int `json:"group_ids"`
	Mode       int   `json:"mode"`
}

type TalkMessageForwardService struct {
	*BaseService
}

func NewTalkMessageForwardService(base *BaseService) *TalkMessageForwardService {
	return &TalkMessageForwardService{base}
}

// 验证消息转发
func (t *TalkMessageForwardService) verifyForward(forward *ForwardParams) error {

	query := t.db.Model(&model.TalkRecords{})

	query.Where("id in ?", forward.RecordsIds)

	if forward.TalkType == entity.PrivateChat {
		subWhere := t.db.Where("user_id = ? and receiver_id = ?", forward.UserId, forward.ReceiverId)
		subWhere.Or("user_id = ? and receiver_id = ?", forward.ReceiverId, forward.UserId)
		query.Where(subWhere)
	}

	query.Where("talk_type = ?", forward.TalkType)
	query.Where("msg_type in ?", []int{1, 2, 4})
	query.Where("is_revoke = ?", 0)

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}

	if int(count) != len(forward.RecordsIds) {
		return errors.New("转发消息异常")
	}

	return nil
}

// SendForwardMessage 推送转发消息
func (t *TalkMessageForwardService) SendForwardMessage(ctx context.Context, forward *ForwardParams) error {
	var (
		err   error
		items []*PushReceive
	)

	if err = t.verifyForward(forward); err != nil {
		return err
	}

	if forward.Mode == 2 {
		items, err = t.MultiMergeForward(ctx, forward)
	} else {
		items, err = t.MultiSplitForward(ctx, forward)
	}

	for _, item := range items {
		body := entity.JsonText{
			"event": entity.EventTalk,
			"data": entity.JsonText{
				"sender_id":   int64(forward.UserId),
				"receiver_id": int64(item.ReceiverId),
				"talk_type":   item.TalkType,
				"record_id":   int64(item.RecordId),
			}.Json(),
		}

		t.rds.Publish(ctx, entity.SubscribeWsGatewayAll, body.Json())
	}

	return nil
}

type Receives struct {
	ReceiverId int `json:"receiver_id"`
	TalkType   int `json:"talk_type"`
}

type PushReceive struct {
	RecordId   int `json:"record_id"`
	ReceiverId int `json:"receiver_id"`
	TalkType   int `json:"talk_type"`
}

type ForwardMsgItem struct {
	MsgType  int    `json:"msg_type"`
	Content  string `json:"content"`
	Nickname string `json:"nickname"`
}

// 聚合转发数据
func (t *TalkMessageForwardService) aggregation(ctx context.Context, forward *ForwardParams) (string, error) {
	rows := make([]*ForwardMsgItem, 0)
	query := t.db.Table("talk_records")
	query.Joins("left join users on users.id = talk_records.user_id")
	query.Where("talk_records.id in ?", forward.RecordsIds[:3])

	if err := query.Limit(3).Scan(&rows).Error; err != nil {
		return "", err
	}

	data := make([]map[string]interface{}, 0)
	for _, row := range rows {
		item := map[string]interface{}{}

		switch row.MsgType {
		case entity.MsgTypeText:
			text := strings.TrimSpace(row.Content)
			item["nickname"] = row.Nickname
			item["text"] = strutil.MtSubstr(&text, 0, 30)
		case entity.MsgTypeCode:
			item["nickname"] = row.Nickname
			item["text"] = "【代码消息】"
		case entity.MsgTypeFile:
			item["nickname"] = row.Nickname
			item["text"] = "【文件消息】"
		}

		data = append(data, item)
	}

	return jsonutil.JsonEncode(data), nil
}

// MultiMergeForward 转发消息（多条合并转发）
func (t *TalkMessageForwardService) MultiMergeForward(ctx context.Context, forward *ForwardParams) ([]*PushReceive, error) {
	var (
		receives = make([]*Receives, 0)
		arr      = make([]*PushReceive, 0)
	)

	for _, uid := range forward.UserIds {
		receives = append(receives, &Receives{uid, 1})
	}

	for _, gid := range forward.GroupIds {
		receives = append(receives, &Receives{gid, 2})
	}

	text, err := t.aggregation(ctx, forward)
	if err != nil {
		return nil, err
	}

	str := slice.IntToIds(forward.RecordsIds)
	err = t.db.Transaction(func(tx *gorm.DB) error {
		forwards := make([]*model.TalkRecordsForward, 0)
		records := make([]*model.TalkRecords, 0)

		for _, receive := range receives {
			records = append(records, &model.TalkRecords{
				TalkType:   receive.TalkType,
				MsgType:    entity.MsgTypeForward,
				UserId:     forward.UserId,
				ReceiverId: receive.ReceiverId,
			})
		}

		if err := tx.Create(records).Error; err != nil {
			return err
		}

		for _, record := range records {
			forwards = append(forwards, &model.TalkRecordsForward{
				RecordId:  record.Id,
				UserId:    record.UserId,
				RecordsId: str,
				Text:      text,
			})

			arr = append(arr, &PushReceive{
				RecordId:   record.Id,
				ReceiverId: record.ReceiverId,
				TalkType:   record.TalkType,
			})
		}

		if err := tx.Create(&forwards).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return arr, nil
}

// MultiSplitForward 转发消息（多条拆分转发）
func (t *TalkMessageForwardService) MultiSplitForward(ctx context.Context, forward *ForwardParams) ([]*PushReceive, error) {
	var (
		receives  = make([]*Receives, 0)
		arr       = make([]*PushReceive, 0)
		records   = make([]*model.TalkRecords, 0)
		hashFiles = make(map[int]*model.TalkRecordsFile, 0)
		hashCodes = make(map[int]*model.TalkRecordsCode, 0)
	)

	for _, uid := range forward.UserIds {
		receives = append(receives, &Receives{uid, 1})
	}

	for _, gid := range forward.GroupIds {
		receives = append(receives, &Receives{gid, 2})
	}

	if err := t.db.Model(&model.TalkRecords{}).Where("id IN ?", forward.RecordsIds).Scan(&records).Error; err != nil {
		return nil, err
	}

	codeIds, fileIds := make([]int, 0), make([]int, 0)

	for _, record := range records {
		switch record.MsgType {
		case entity.MsgTypeFile:
			fileIds = append(fileIds, record.Id)
		case entity.MsgTypeCode:
			codeIds = append(codeIds, record.Id)
		}
	}

	if len(codeIds) > 0 {
		items := make([]*model.TalkRecordsCode, 0)
		if err := t.db.Model(&model.TalkRecordsCode{}).Where("record_id IN ?", codeIds).Scan(&items).Error; err == nil {
			for i := range items {
				hashCodes[items[i].RecordId] = items[i]
			}
		}
	}

	if len(fileIds) > 0 {
		items := make([]*model.TalkRecordsFile, 0)
		if err := t.db.Model(&model.TalkRecordsFile{}).Where("record_id IN ?", fileIds).Scan(&items).Error; err == nil {
			for i := range items {
				hashFiles[items[i].RecordId] = items[i]
			}
		}
	}

	err := t.db.Transaction(func(tx *gorm.DB) error {
		for _, item := range records {
			items := make([]*model.TalkRecords, 0)
			files := make([]*model.TalkRecordsFile, 0)
			codes := make([]*model.TalkRecordsCode, 0)

			for _, receive := range receives {
				items = append(items, &model.TalkRecords{
					TalkType:   receive.TalkType,
					MsgType:    item.MsgType,
					UserId:     forward.UserId,
					ReceiverId: receive.ReceiverId,
					Content:    item.Content,
				})
			}

			if err := tx.Create(items).Error; err != nil {
				return err
			}

			for _, record := range items {
				arr = append(arr, &PushReceive{
					RecordId:   record.Id,
					ReceiverId: record.ReceiverId,
					TalkType:   record.TalkType,
				})

				switch record.MsgType {
				case entity.MsgTypeFile:
					if file, ok := hashFiles[item.Id]; ok {
						files = append(files, &model.TalkRecordsFile{
							RecordId:     record.Id,
							UserId:       forward.UserId,
							FileSource:   file.FileSource,
							FileType:     file.FileType,
							SaveType:     file.SaveType,
							OriginalName: file.OriginalName,
							FileSuffix:   file.FileSuffix,
							FileSize:     file.FileSize,
							SaveDir:      file.SaveDir,
						})
					}
				case entity.MsgTypeCode:
					if code, ok := hashCodes[item.Id]; ok {
						codes = append(codes, &model.TalkRecordsCode{
							RecordId: record.Id,
							UserId:   forward.UserId,
							CodeLang: code.CodeLang,
							Code:     code.Code,
						})
					}
				}
			}

			if len(files) > 0 {
				if err := tx.Create(files).Error; err != nil {
					return err
				}
			}

			if len(codes) > 0 {
				if err := tx.Create(codes).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return arr, nil
}
