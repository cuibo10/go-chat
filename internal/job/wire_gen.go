// Code generated by Wire. DO NOT EDIT.

//go:generate go run github.com/google/wire/cmd/wire
//+build !wireinject

package main

import (
	"context"
	"github.com/google/wire"
	"go-chat/internal/dao"
	"go-chat/internal/dao/note"
	"go-chat/internal/job/internal/cmd"
	crontab2 "go-chat/internal/job/internal/cmd/crontab"
	"go-chat/internal/job/internal/cmd/other"
	"go-chat/internal/job/internal/cmd/queue"
	"go-chat/internal/job/internal/handle/crontab"
	"go-chat/internal/pkg/client"
	"go-chat/internal/pkg/filesystem"
	"go-chat/internal/provider"
)

// Injectors from wire.go:

func Initialize(ctx context.Context) *Providers {
	config := provider.NewConfig()
	db := provider.NewMySQLClient(config)
	client := provider.NewRedisClient(ctx, config)
	baseDao := dao.NewBaseDao(db, client)
	splitUploadDao := dao.NewFileSplitUploadDao(baseDao)
	filesystemFilesystem := filesystem.NewFilesystem(config)
	clearTmpFile := crontab.NewClearTmpFile(splitUploadDao, filesystemFilesystem)
	clearTmpFileCommand := crontab2.NewClearTmpFileCommand(clearTmpFile)
	articleAnnexDao := note.NewArticleAnnexDao(baseDao)
	clearArticle := crontab.NewClearArticle(articleAnnexDao, filesystemFilesystem)
	clearArticleCommand := crontab2.NewClearArticleCommand(clearArticle)
	crontabCommand := crontab2.NewCrontabCommand(clearTmpFileCommand, clearArticleCommand)
	queueCommand := queue.NewQueueCommand()
	otherCommand := other.NewOtherCommand()
	commands := &cmd.Commands{
		CrontabCommand: crontabCommand,
		QueueCommand:   queueCommand,
		OtherCommand:   otherCommand,
	}
	providers := &Providers{
		Config:   config,
		Commands: commands,
	}
	return providers
}

// wire.go:

var providerSet = wire.NewSet(provider.NewConfig, provider.NewMySQLClient, provider.NewRedisClient, provider.NewHttpClient, client.NewHttpClient, filesystem.NewFilesystem, dao.NewBaseDao, dao.NewFileSplitUploadDao, note.NewArticleAnnexDao, note.NewArticleDao, crontab2.NewCrontabCommand, queue.NewQueueCommand, other.NewOtherCommand, crontab2.NewClearTmpFileCommand, crontab2.NewClearArticleCommand, crontab.NewClearTmpFile, crontab.NewClearArticle, wire.Struct(new(cmd.Commands), "*"), wire.Struct(new(Providers), "*"))
