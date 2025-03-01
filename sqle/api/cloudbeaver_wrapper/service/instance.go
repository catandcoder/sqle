package service

import (
	"context"
	"fmt"
	gqlClient "github.com/actiontech/sqle/sqle/api/cloudbeaver_wrapper/graph/client"
	"github.com/actiontech/sqle/sqle/driver"
	sqleModel "github.com/actiontech/sqle/sqle/model"
)

func SyncUserBindInstance(cbUserID string) error {
	// 获取当前SQLE用户
	s := sqleModel.GetStorage()
	sqleUserCache, exist, err := s.GetCloudBeaverUserCacheByCBUserID(cbUserID)
	if err != nil || !exist { // 如果用户不存在表示这可能是个与SQLE无关的用户
		return err
	}
	sqleUser, exist, err := s.GetUserByID(sqleUserCache.SQLEUserID)
	if err != nil || !exist {
		return err
	}

	// 获取用户当前拥有权限的SQLE实例
	var sqleInstSlice []*sqleModel.Instance
	if sqleUser.Name == sqleModel.DefaultAdminUser {
		sqleInstSlice, err = s.GetAllInstance()
	} else {
		sqleInstSlice, err = s.GetUserCanOpInstances(sqleUser, []uint{sqleModel.OP_SQL_QUERY_QUERY})
	}
	if err != nil {
		return err
	}

	sqleInstMap := map[uint] /*sqle inst id*/ *sqleModel.Instance{}
	for _, instance := range sqleInstSlice {
		inst, _, err := s.GetInstanceById(fmt.Sprintf("%v", instance.ID))
		if err != nil {
			return err
		}
		sqleInstMap[instance.ID] = inst
	}

	// 同步实例信息
	if err := SyncInstance(sqleInstMap); err != nil {
		return err
	}

	// 同步权限信息
	if err := SyncVisibleInstance(sqleUserCache, sqleUser, sqleInstMap); err != nil {
		return err
	}

	return nil
}

func SyncInstance(sqleInstances map[uint] /*sqle inst id*/ *sqleModel.Instance) error {
	ids := []uint{}
	for _, instance := range sqleInstances {
		ids = append(ids, instance.ID)
	}

	// 从缓存中获取需要同步的CloudBeaver实例
	s := sqleModel.GetStorage()
	cbInstCacheSlice, err := s.GetCloudBeaverInstanceCacheBySQLEInstIDs(ids)
	if err != nil {
		return err
	}
	cbInstCacheMap := map[uint] /* sqle inst id*/ *sqleModel.CloudBeaverInstanceCache{}
	for _, cache := range cbInstCacheSlice {
		cbInstCacheMap[cache.SQLEInstanceID] = cache
	}

	// 找到需要同步的实例
	needAdd := []uint /*sqle inst id*/ {}
	needUpdate := []uint /*sqle inst id*/ {}
	for sqleInstID, sqleInst := range sqleInstances {
		cb, ok := cbInstCacheMap[sqleInstID]
		if !ok {
			needAdd = append(needAdd, sqleInstID)
		} else if cb.SQLEInstanceFingerprint != sqleInst.Fingerprint() {
			needUpdate = append(needUpdate, sqleInstID)
		}
	}

	if len(needAdd) == 0 && len(needUpdate) == 0 {
		return nil
	}

	// 获取管理员链接
	client, err := GetGQLClientWithRootUser()
	if err != nil {
		return err
	}

	// 同步实例信息
	for _, sqleInstID := range needAdd {
		if err := AddCloudBeaverInstance(client, sqleInstances[sqleInstID]); err != nil {
			return err
		}
	}
	for _, sqleInstID := range needUpdate {
		if err := UpdateCloudBeaverInstance(client, cbInstCacheMap[sqleInstID].CloudBeaverInstanceID, sqleInstances[sqleInstID]); err != nil {
			return err
		}
	}

	return nil

}

// AddCloudBeaverInstance 添加实例后会同步缓存
func AddCloudBeaverInstance(client *gqlClient.Client, sqleInst *sqleModel.Instance) error {
	driverID, err := GenerateCloudBeaverDriverID(sqleInst)
	if err != nil {
		fmt.Println("Instances of this type are not currently supported:", sqleInst.DbType)
		// 不支持的类型跳过就好,没必要终端流程
		//nolint:nilerr
		return nil
	}
	// 添加实例
	req := gqlClient.NewRequest(CreateConnectionQuery, map[string]interface{}{
		"projectId": "g_GlobalConfiguration",
		"config": map[string]interface{}{
			"configurationType": "MANUAL",
			"name":              sqleInst.Name,
			"template":          false,
			"driverId":          driverID,
			"host":              sqleInst.Host,
			"port":              sqleInst.Port,
			"authModelId":       "native",
			"saveCredentials":   true,
			"credentials": map[string]interface{}{
				"userName":     sqleInst.User,
				"userPassword": sqleInst.Password,
			},
			"properties": map[string]interface{}{},
		},
	})
	resp := struct {
		Connection struct {
			ID string `json:"id"`
		} `json:"connection"`
	}{}

	err = client.Run(context.TODO(), req, &resp)
	if err != nil {
		return err
	}

	// 同步缓存
	s := sqleModel.GetStorage()
	return s.Save(&sqleModel.CloudBeaverInstanceCache{
		CloudBeaverInstanceID:   resp.Connection.ID,
		SQLEInstanceID:          sqleInst.ID,
		SQLEInstanceFingerprint: sqleInst.Fingerprint(),
	})
}

func GenerateCloudBeaverDriverID(sqleInst *sqleModel.Instance) (string, error) {
	switch sqleInst.DbType {
	case driver.DriverTypeMySQL:
		return "mysql:mysql8", nil
	default:
		return "", fmt.Errorf("temporarily unsupported instance types")
	}
}

const (
	CreateConnectionQuery = `
mutation createConnection(
  $projectId: ID!
  $config: ConnectionConfig!
) {
  connection: createConnection(projectId: $projectId, config: $config) {
    ...DatabaseConnection
  }
}

fragment DatabaseConnection on ConnectionInfo {
  id
}
`
)

// UpdateCloudBeaverInstance 更新完毕后会同步缓存
func UpdateCloudBeaverInstance(client *gqlClient.Client, cbInstID string, sqleInst *sqleModel.Instance) error {
	driverID, err := GenerateCloudBeaverDriverID(sqleInst)
	if err != nil {
		fmt.Println("Instances of this type are not currently supported:", sqleInst.DbType)
		// 不支持的类型跳过就好,没必要终端流程
		//nolint:nilerr
		return nil
	}
	// 更新实例
	req := gqlClient.NewRequest(UpdateConnectionQuery, map[string]interface{}{
		"projectId": "g_GlobalConfiguration",
		"config": map[string]interface{}{
			"connectionId":      cbInstID,
			"configurationType": "MANUAL",
			"name":              sqleInst.Name,
			"template":          false,
			"driverId":          driverID,
			"host":              sqleInst.Host,
			"port":              sqleInst.Port,
			"databaseName":      nil,
			"authModelId":       "native",
			"saveCredentials":   true,
			"credentials": map[string]interface{}{
				"userName":     sqleInst.User,
				"userPassword": sqleInst.Password,
			},
			"properties": map[string]interface{}{},
		},
	})
	resp := struct {
		Connection struct {
			ID string `json:"id"`
		} `json:"connection"`
	}{}

	err = client.Run(context.TODO(), req, &resp)
	if err != nil {
		return err
	}

	// 同步缓存
	s := sqleModel.GetStorage()
	return s.Save(&sqleModel.CloudBeaverInstanceCache{
		CloudBeaverInstanceID:   resp.Connection.ID,
		SQLEInstanceID:          sqleInst.ID,
		SQLEInstanceFingerprint: sqleInst.Fingerprint(),
	})
}

const (
	UpdateConnectionQuery = `
mutation updateConnection(
  $projectId: ID!
  $config: ConnectionConfig!
) {
  connection: updateConnection(projectId: $projectId, config: $config) {
    ...DatabaseConnection
  }
}

fragment DatabaseConnection on ConnectionInfo {
  id
}
`
)

func SyncVisibleInstance(cbUserCache *sqleModel.CloudBeaverUserCache, sqleUser *sqleModel.User, sqleInstances map[uint] /*sqle inst id*/ *sqleModel.Instance) error {

	if cbUserCache.SQLEFingerprint != sqleUser.FingerPrint() {
		return fmt.Errorf("user information is not synchronized, unable to update instance information")
	}

	ids := []uint{}
	for _, instance := range sqleInstances {
		ids = append(ids, instance.ID)
	}

	// 从缓存中获取需要同步的CloudBeaver实例
	s := sqleModel.GetStorage()
	cbInstCacheSlice, err := s.GetCloudBeaverInstanceCacheBySQLEInstIDs(ids)
	if err != nil {
		return err
	}
	cbInstCacheMap := map[string] /* cb inst id*/ *sqleModel.CloudBeaverInstanceCache{}
	for _, cache := range cbInstCacheSlice {
		cbInstCacheMap[cache.CloudBeaverInstanceID] = cache
	}

	// 获取用户当前实例列表
	getConnResp := &struct {
		Connections []*struct {
			Id string `json:"id"`
		} `json:"connections"`
	}{}

	getConReq := gqlClient.NewRequest(GetUserConnectionsQuery, nil)

	client, err := GetGQLClient(cbUserCache.CloudBeaverUserID, sqleUser.Password)
	if err != nil {
		return err
	}

	err = client.Run(context.TODO(), getConReq, getConnResp)
	if err != nil {
		return err
	}

	// 判断是否需要同步权限
	if len(getConnResp.Connections) != len(cbInstCacheSlice) {
		return syncVisibleInstance(cbInstCacheSlice, cbUserCache.CloudBeaverUserID)
	}
	for _, connection := range getConnResp.Connections {
		if _, ok := cbInstCacheMap[connection.Id]; !ok {
			return syncVisibleInstance(cbInstCacheSlice, cbUserCache.CloudBeaverUserID)
		}
	}
	return nil

}

func syncVisibleInstance(cbInstCacheSlice []*sqleModel.CloudBeaverInstanceCache, cloudBeaverUserID string) error {
	cbInstIDs := []string{}
	for _, cache := range cbInstCacheSlice {
		cbInstIDs = append(cbInstIDs, cache.CloudBeaverInstanceID)
	}
	setConnReq := gqlClient.NewRequest(SetUserConnectionsQuery, map[string]interface{}{
		"userId":      cloudBeaverUserID,
		"connections": cbInstIDs,
	})
	rootClient, err := GetGQLClientWithRootUser()
	if err != nil {
		return err
	}
	err = rootClient.Run(context.TODO(), setConnReq, nil)
	return err
}

const (
	GetUserConnectionsQuery = `
query getUserConnections (
  $projectId: ID
  $connectionId: ID
){
  connections: userConnections(projectId: $projectId, id: $connectionId) {
    ...DatabaseConnection
  }
}

fragment DatabaseConnection on ConnectionInfo {
  id
}
`
	SetUserConnectionsQuery = `
query setConnections($userId: ID!, $connections: [ID!]!) {
  grantedConnections: setSubjectConnectionAccess(
    subjectId: $userId
    connections: $connections
  )
}
`
)
