package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// ModelRoute is an explicitly managed logical-to-physical model mapping.
// Ordered members express preference; prices in different channels are not comparable.
type ModelRoute struct {
	ID       int64    `json:"id"`
	Model    string   `json:"model"`
	Name     string   `json:"name"`
	Enabled  bool     `json:"enabled"`
	Members  []string `json:"members"`
	Revision int64    `json:"revision"`
}

func cloneModelRoutes(routes []ModelRoute) []ModelRoute {
	out := append([]ModelRoute{}, routes...)
	for i := range out {
		out[i].Members = append([]string{}, out[i].Members...)
	}
	return out
}

func (d *DB) ModelRoutes() ([]ModelRoute, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.routesCache != nil {
		return cloneModelRoutes(d.routesCache), nil
	}
	rows, err := d.db.Query("SELECT id,model,name,enabled,members,revision FROM model_routes ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ModelRoute{}
	for rows.Next() {
		var r ModelRoute
		var raw string
		if err := rows.Scan(&r.ID, &r.Model, &r.Name, &r.Enabled, &raw, &r.Revision); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &r.Members); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	d.routesCache = cloneModelRoutes(out)
	return out, nil
}

func (d *DB) SaveModelRoute(r ModelRoute) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	b, err := json.Marshal(r.Members)
	if err != nil {
		return 0, err
	}
	if r.ID == 0 {
		res, err := d.db.Exec("INSERT INTO model_routes(model,name,enabled,members,revision) VALUES(?,?,?,?,1)", r.Model, r.Name, r.Enabled, string(b))
		if err != nil {
			return 0, err
		}
		d.routesCache = nil
		return res.LastInsertId()
	}
	res, err := d.db.Exec("UPDATE model_routes SET name=?,enabled=?,members=?,revision=revision+1 WHERE id=? AND model=? AND revision=?", r.Name, r.Enabled, string(b), r.ID, r.Model, r.Revision)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n != 1 {
		return 0, errors.New("配置已变更或模型名称被修改，请重新加载")
	}
	d.routesCache = nil
	return r.ID, nil
}

func (d *DB) DeleteModelRoute(id, revision int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	res, err := d.db.Exec("DELETE FROM model_routes WHERE id=? AND revision=?", id, revision)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("配置已变更，请重新加载")
	}
	d.routesCache = nil
	return nil
}

// Grants freeze the members approved when a restricted key's allowlist is saved.
// Editing a route cannot grant that key a newly added channel. Deletion cascades
// grants so recreating a route with the same public name does not resurrect them.
func captureRouteGrants(tx *sql.Tx, appID int64, allowed string) error {
	if _, err := tx.Exec("DELETE FROM app_route_grants WHERE app_id=?", appID); err != nil {
		return err
	}
	for _, model := range parseAllowedModels(allowed) {
		var id int64
		var members string
		err := tx.QueryRow("SELECT id,members FROM model_routes WHERE model=?", model).Scan(&id, &members)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT INTO app_route_grants(app_id,route_id,members) VALUES(?,?,?)", appID, id, members); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) AppRouteMembers(appID, routeID int64) (map[string]bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var raw string
	err := d.db.QueryRow("SELECT members FROM app_route_grants WHERE app_id=? AND route_id=?", appID, routeID).Scan(&raw)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	out := map[string]bool{}
	if err == sql.ErrNoRows {
		return out, nil
	}
	var members []string
	if err = json.Unmarshal([]byte(raw), &members); err != nil {
		return nil, fmt.Errorf("模型授权快照无效")
	}
	for _, m := range members {
		out[m] = true
	}
	return out, nil
}
