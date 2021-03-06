/*
 * Copyright 2019 The CovenantSQL Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	gorp "gopkg.in/gorp.v2"

	"github.com/CovenantSQL/CovenantSQL/client"
	"github.com/CovenantSQL/CovenantSQL/cmd/cql-proxy/config"
	"github.com/CovenantSQL/CovenantSQL/cmd/cql-proxy/model"
	"github.com/CovenantSQL/CovenantSQL/cmd/cql-proxy/utils"
	"github.com/CovenantSQL/CovenantSQL/route"
	rpc "github.com/CovenantSQL/CovenantSQL/rpc/mux"
	"github.com/CovenantSQL/CovenantSQL/types"
)

func applyToken(c *gin.Context) {
	var (
		amount        uint64
		userLimits    int64
		accountLimits int64
	)

	cfg := getConfig(c)
	if cfg != nil && cfg.Faucet != nil && cfg.Faucet.Enabled {
		amount = cfg.Faucet.Amount
		userLimits = cfg.Faucet.AccountDailyQuota
		accountLimits = cfg.Faucet.AddressDailyQuota
	} else {
		abortWithError(c, http.StatusForbidden, ErrTokenApplyDisabled)
		return
	}

	developer := getDeveloperID(c)
	p, err := model.GetMainAccount(model.GetDB(c), developer)
	if err != nil {
		_ = c.Error(err)
		abortWithError(c, http.StatusBadRequest, ErrNoMainAccount)
		return
	}

	err = model.CheckTokenApplyLimits(model.GetDB(c), developer, p.Account, userLimits, accountLimits)
	if err != nil {
		_ = c.Error(err)
		abortWithError(c, http.StatusInternalServerError, ErrTokenApplyLimitExceeded)
		return
	}

	// run task
	taskID, err := getTaskManager(c).New(model.TaskApplyToken, developer, p.ID, gin.H{
		"amount": amount,
	})
	if err != nil {
		_ = c.Error(err)
		abortWithError(c, http.StatusInternalServerError, ErrCreateTaskFailed)
		return
	}

	responseWithData(c, http.StatusOK, gin.H{
		"task_id": taskID,
		"amount":  amount,
	})
}

func showAllAccounts(c *gin.Context) {
	developer := getDeveloperID(c)
	d, err := model.GetDeveloper(model.GetDB(c), developer)
	if err != nil {
		_ = c.Error(err)
		abortWithError(c, http.StatusForbidden, ErrInvalidDeveloper)
		return
	}

	accounts, err := model.GetAllAccounts(model.GetDB(c), developer)
	if err != nil {
		_ = c.Error(err)
		abortWithError(c, http.StatusInternalServerError, ErrGetAccountFailed)
		return
	}

	var (
		apiResp = gin.H{}
		keys    []gin.H
	)

	for _, account := range accounts {
		var (
			req     = new(types.QueryAccountTokenBalanceReq)
			resp    = new(types.QueryAccountTokenBalanceResp)
			keyData = gin.H{}
		)

		req.Addr, err = account.Account.Get()
		if err != nil {
			_ = c.Error(err)
			abortWithError(c, http.StatusBadRequest, ErrParseAccountFailed)
			return
		}

		if account.ID == d.MainAccount {
			apiResp["main"] = req.Addr.String()
		}

		keyData["account"] = req.Addr.String()

		if err = rpc.RequestBP(route.MCCQueryAccountTokenBalance.String(), req, resp); err == nil {
			keyData["balance"] = resp.Balance
		} else {
			err = nil
		}

		keys = append(keys, keyData)
	}

	apiResp["keypairs"] = keys

	responseWithData(c, http.StatusOK, apiResp)
}

func getBalance(c *gin.Context) {
	developer := getDeveloperID(c)
	p, err := model.GetMainAccount(model.GetDB(c), developer)
	if err != nil {
		_ = c.Error(err)
		abortWithError(c, http.StatusForbidden, ErrNoMainAccount)
		return
	}

	var (
		req  = new(types.QueryAccountTokenBalanceReq)
		resp = new(types.QueryAccountTokenBalanceResp)
	)

	req.Addr, err = p.Account.Get()
	if err != nil {
		_ = c.Error(err)
		abortWithError(c, http.StatusBadRequest, ErrParseAccountFailed)
		return
	}

	if err = rpc.RequestBP(route.MCCQueryAccountTokenBalance.String(), req, resp); err != nil {
		_ = c.Error(err)
		abortWithError(c, http.StatusInternalServerError, ErrSendETLSRPCFailed)
		return
	}

	responseWithData(c, http.StatusOK, gin.H{
		"balance": resp.Balance,
	})
}

func setMainAccount(c *gin.Context) {
	r := struct {
		Account utils.AccountAddress `json:"account" form:"account" binding:"required,len=64"`
	}{}

	if err := c.ShouldBind(&r); err != nil {
		abortWithError(c, http.StatusBadRequest, err)
		return
	}

	developer := getDeveloperID(c)
	err := model.SetMainAccount(model.GetDB(c), developer, r.Account)
	if err != nil {
		_ = c.Error(err)
		abortWithError(c, http.StatusInternalServerError, ErrSetMainAccountFailed)
		return
	}

	responseWithData(c, http.StatusOK, nil)

	return
}

// ApplyTokenTask handles the token apply process.
func ApplyTokenTask(ctx context.Context, cfg *config.Config, db *gorp.DbMap, t *model.Task) (r gin.H, err error) {
	args := struct {
		Amount uint64 `json:"amount"`
	}{}
	err = json.Unmarshal(t.RawArgs, &args)
	if err != nil {
		err = errors.Wrapf(err, "unmarshal task args failed")
		return
	}

	p, err := model.GetAccountByID(db, t.Developer, t.Account)
	if err != nil {
		err = errors.Wrapf(err, "get account for task failed")
		return
	}

	accountAddr, err := p.Account.Get()
	if err != nil {
		err = errors.Wrapf(err, "decode task account failed")
		return
	}

	txHash, err := client.TransferToken(accountAddr, args.Amount, types.Particle)
	if err != nil {
		err = errors.Wrapf(err, "send transfer token rpc failed")
		return
	}

	// add record
	ar, err := model.AddTokenApplyRecord(db, t.Developer, p.Account, args.Amount)
	if err != nil {
		err = errors.Wrapf(err, "record token application log failed")
		return
	}

	// wait for transaction to complete in several cycles
	timeoutCtx, cancelCtx := context.WithTimeout(ctx, 3*time.Minute)
	defer cancelCtx()

	lastState, _ := waitForTxState(timeoutCtx, txHash)
	r = gin.H{
		"id":      ar.ID,
		"account": p.Account,
		"tx":      txHash.String(),
		"state":   lastState.String(),
	}

	return
}
