// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2017-2018 Canonical Ltd
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package controller

import (
	"github.com/gorilla/mux"
)

// TODO: need to add support for graceful shutdown

// A Daemon listens for requests and routes them to the right command
type Mux struct {
	initialized   bool
	router        *mux.Router
}

func (m *Mux) Init() {
	m.router = mux.NewRouter()
	initCommand(m.router)
	initStatus(m.router)
	initService(m.router)
	initUpdate(m.router)
}

// New Mux
// TODO: re-factor to make this a singleton
func New() (*Mux, error) {
	return &Mux{}, nil
}
