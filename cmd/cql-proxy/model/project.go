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

package model

import (
	"github.com/pkg/errors"
	gorp "gopkg.in/gorp.v2"

	"github.com/CovenantSQL/CovenantSQL/crypto/hash"
	"github.com/CovenantSQL/CovenantSQL/proto"
)

type Project struct {
	ID        int64            `db:"id"`
	DB        proto.DatabaseID `db:"database_id"`
	Alias     string           `db:"alias"`
	Developer int64            `db:"developer_id"`
	Account   int64            `db:"account_id"`
}

func AddProject(db *gorp.DbMap, dbID proto.DatabaseID, developer int64, account int64) (p *Project, err error) {
	p = &Project{
		DB:        dbID,
		Alias:     string(dbID)[:8],
		Developer: developer,
		Account:   account,
	}
	err = db.Insert(p)
	if err != nil {
		err = errors.Wrapf(err, "add project failed")
	}
	return
}

func GetProject(db *gorp.DbMap, name string) (p *Project, err error) {
	// if the alias fits to a hash, query using database id
	var h hash.Hash
	err = hash.Decode(&h, name)
	if err == nil {
		err = db.SelectOne(&p, `SELECT * FROM "project" WHERE "database_id" = ? LIMIT 1`, name)
	}

	if err == nil {
		return
	}

	err = db.SelectOne(&p, `SELECT * FROM "project" WHERE "alias" = ? LIMIT 1`, name)
	if err != nil {
		err = errors.Wrapf(err, "get project failed")
	}
	return
}

func GetProjectByID(db *gorp.DbMap, dbID proto.DatabaseID, developer int64) (p *Project, err error) {
	err = db.SelectOne(&p,
		`SELECT * FROM "project" WHERE "database_id" = ? AND "developer_id" = ? LIMIT 1`,
		dbID, developer)
	if err != nil {
		err = errors.Wrapf(err, "get project failed")
	}
	return
}

func GetProjects(db *gorp.DbMap, developer int64, account int64) (p []*Project, err error) {
	if account == 0 {
		_, err = db.Select(&p, `SELECT * FROM "project" WHERE "developer_id" = ?`, developer)
	} else {
		_, err = db.Select(&p, `SELECT * FROM "project" WHERE "developer_id" = ? AND "account_id" = ?`,
			developer, account)
	}
	if err != nil {
		err = errors.Wrapf(err, "get projects failed")
	}
	return
}

func DeleteProject(db *gorp.DbMap, dbID proto.DatabaseID, developer int64) (err error) {
	_, err = db.Exec(
		`DELETE FROM "project" WHERE "database_id" = ? AND "developer_id" = ?`,
		dbID, developer)
	if err != nil {
		err = errors.Wrapf(err, "delete project failed")
	}
	return
}

func SetProjectAlias(db *gorp.DbMap, dbID proto.DatabaseID, developer int64, alias string) (err error) {
	if alias == "" {
		alias = string(dbID)
	}
	_, err = db.Exec(
		`UPDATE "project" SET "alias" = ? WHERE "database_id" = ? AND "developer_id" = ?`,
		alias, dbID, developer)
	if err != nil {
		err = errors.Wrapf(err, "set project alias failed")
	}
	return
}
