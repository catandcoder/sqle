package v1

import (
	_errors "errors"
	"fmt"
	"net/http"

	"github.com/actiontech/sqle/sqle/api/controller"
	"github.com/actiontech/sqle/sqle/errors"
	"github.com/actiontech/sqle/sqle/model"
	"github.com/labstack/echo/v4"
)

type CreateUserReqV1 struct {
	Name       string   `json:"user_name" form:"user_name" example:"test" valid:"required,name"`
	Password   string   `json:"user_password" form:"user_name" example:"123456" valid:"required"`
	Email      string   `json:"email" form:"email" example:"test@email.com" valid:"omitempty,email"`
	WeChatID   string   `json:"wechat_id" example:"UserID"`
	Roles      []string `json:"role_name_list" form:"role_name_list"`
	UserGroups []string `json:"user_group_name_list" form:"user_group_name_list"`
}

// @Summary 创建用户
// @Description create user
// @Id createUserV1
// @Tags user
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param instance body v1.CreateUserReqV1 true "create user"
// @Success 200 {object} controller.BaseRes
// @router /v1/users [post]
func CreateUser(c echo.Context) error {
	req := new(CreateUserReqV1)
	if err := controller.BindAndValidateReq(c, req); err != nil {
		return err
	}

	s := model.GetStorage()
	_, exist, err := s.GetUserByName(req.Name)
	if err != nil {
		return controller.JSONBaseErrorReq(c, err)
	}
	if exist {
		return controller.JSONBaseErrorReq(c, errors.New(errors.DataExist, fmt.Errorf("user is exist")))
	}

	var roles []*model.Role
	{
		if req.Roles != nil || len(req.Roles) > 0 {
			roles, err = s.GetAndCheckRoleExist(req.Roles)
			if err != nil {
				return controller.JSONBaseErrorReq(c, err)
			}
		}
	}

	var userGroups []*model.UserGroup
	{
		if req.UserGroups != nil || len(req.UserGroups) > 0 {
			userGroups, err = s.GetAndCheckUserGroupExist(req.UserGroups)
			if err != nil {
				return controller.JSONBaseErrorReq(c, err)
			}
		}
	}

	user := &model.User{
		Name:     req.Name,
		Password: req.Password,
		Email:    req.Email,
		WeChatID: req.WeChatID,
	}

	return controller.JSONBaseErrorReq(c,
		s.SaveUserAndAssociations(user, roles, userGroups))
}

type UpdateUserReqV1 struct {
	Email      *string   `json:"email" valid:"omitempty,len=0|email" form:"email"`
	WeChatID   *string   `json:"wechat_id" example:"UserID"`
	Roles      *[]string `json:"role_name_list" form:"role_name_list"`
	IsDisabled *bool     `json:"is_disabled,omitempty" form:"is_disabled"`
	UserGroups *[]string `json:"user_group_name_list" form:"user_group_name_list"`
}

// @Summary 更新用户信息
// @Description update user
// @Id updateUserV1
// @Tags user
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param user_name path string true "user name"
// @Param instance body v1.UpdateUserReqV1 true "update user"
// @Success 200 {object} controller.BaseRes
// @router /v1/users/{user_name}/ [patch]
func UpdateUser(c echo.Context) error {
	req := new(UpdateUserReqV1)
	if err := controller.BindAndValidateReq(c, req); err != nil {
		return err
	}
	userName := c.Param("user_name")
	s := model.GetStorage()
	user, exist, err := s.GetUserByName(userName)
	if err != nil {
		return controller.JSONBaseErrorReq(c, err)
	}
	if !exist {
		return controller.JSONBaseErrorReq(c, errors.New(errors.DataNotExist, fmt.Errorf("user is not exist")))
	}

	// Email
	if req.Email != nil {
		user.Email = *req.Email
	}

	// WeChatID
	if req.WeChatID != nil {
		user.WeChatID = *req.WeChatID
	}

	// IsDisabled
	if req.IsDisabled != nil {
		if err := controller.CanThisUserBeDisabled(
			controller.GetUserName(c), userName); err != nil {
			return controller.JSONBaseErrorReq(c, err)
		}
		if *req.IsDisabled {
			user.SetStat(model.Disabled)
		} else {
			user.SetStat(model.Enabled)
		}
	}

	// roles
	var roles []*model.Role
	{

		if req.Roles != nil {
			if len(*req.Roles) > 0 {
				roles, err = s.GetAndCheckRoleExist(*req.Roles)
				if err != nil {
					return controller.JSONBaseErrorReq(c, err)
				}
			} else {
				roles = make([]*model.Role, 0)
			}
		}
	}

	// user_groups
	var userGroups []*model.UserGroup
	{
		if req.UserGroups != nil {
			if len(*req.UserGroups) > 0 {
				userGroups, err = s.GetAndCheckUserGroupExist(*req.UserGroups)
				if err != nil {
					return controller.JSONBaseErrorReq(c, err)
				}
			} else {
				userGroups = make([]*model.UserGroup, 0)
			}
		}

	}

	return controller.JSONBaseErrorReq(c, s.SaveUserAndAssociations(user, roles, userGroups))
}

// @Summary 删除用户
// @Description delete user
// @Id deleteUserV1
// @Tags user
// @Security ApiKeyAuth
// @Param user_name path string true "user name"
// @Success 200 {object} controller.BaseRes
// @router /v1/users/{user_name}/ [delete]
func DeleteUser(c echo.Context) error {
	userName := c.Param("user_name")
	if userName == model.DefaultAdminUser {
		return controller.JSONBaseErrorReq(c, errors.New(errors.DataConflict,
			fmt.Errorf("admin user cannot be deleted")))
	}
	s := model.GetStorage()
	user, exist, err := s.GetUserByName(userName)
	if err != nil {
		return controller.JSONBaseErrorReq(c, err)
	}
	if !exist {
		return controller.JSONBaseErrorReq(c, errors.New(errors.DataNotExist, fmt.Errorf("user is not exist")))
	}

	exist, err = s.UserHasRunningWorkflow(user.ID)
	if err != nil {
		return controller.JSONBaseErrorReq(c, err)
	}
	if exist {
		return controller.JSONBaseErrorReq(c, errors.New(errors.DataExist,
			fmt.Errorf("%s can't be deleted,cause wait_for_audit or wait_for_execution workflow exist", userName)))
	}
	hasBind, err := s.UserHasBindWorkflowTemplate(user)
	if err != nil {
		return controller.JSONBaseErrorReq(c, err)
	}
	if hasBind {
		return controller.JSONBaseErrorReq(c, errors.New(errors.DataExist,
			fmt.Errorf("%s can't be deleted,cause the user binds the workflow template", userName)))
	}

	err = s.Delete(user)
	if err != nil {
		return controller.JSONBaseErrorReq(c, err)
	}
	return controller.JSONBaseErrorReq(c, nil)
}

type UpdateOtherUserPasswordReqV1 struct {
	Password string `json:"password"  valid:"required"`
}

// @Summary admin修改其他用户密码
// @Description admin modifies the passwords of other users
// @Id UpdateOtherUserPasswordV1
// @Tags user
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param user_name path string true "user name"
// @Param instance body v1.UpdateOtherUserPasswordReqV1 true "change user's password"
// @Success 200 {object} controller.BaseRes
// @router /v1/users/{user_name}/password [patch]
func UpdateOtherUserPassword(c echo.Context) error {
	req := new(UpdateOtherUserPasswordReqV1)
	if err := controller.BindAndValidateReq(c, req); err != nil {
		return err
	}
	userName := c.Param("user_name")
	if userName == model.DefaultAdminUser {
		return controller.JSONBaseErrorReq(c, errors.New(errors.DataConflict,
			fmt.Errorf("admin user's password cannot be changed in this page")))
	}

	s := model.GetStorage()
	user, exist, err := s.GetUserByName(userName)
	if err != nil {
		return controller.JSONBaseErrorReq(c, err)
	}
	if !exist {
		return controller.JSONBaseErrorReq(c, errors.New(errors.DataNotExist, fmt.Errorf("user is not exist")))
	}
	if user.UserAuthenticationType == model.UserAuthenticationTypeLDAP {
		return controller.JSONBaseErrorReq(c, errLdapUserCanNotChangePassword)
	}
	err = s.UpdatePassword(user, req.Password)
	if err != nil {
		return controller.JSONBaseErrorReq(c, err)
	}

	return controller.JSONBaseErrorReq(c, nil)
}

type GetUserDetailResV1 struct {
	controller.BaseRes
	Data UserDetailResV1 `json:"data"`
}

type UserDetailResV1 struct {
	Name       string   `json:"user_name"`
	Email      string   `json:"email"`
	IsAdmin    bool     `json:"is_admin"`
	WeChatID   string   `json:"wechat_id"`
	LoginType  string   `json:"login_type"`
	Roles      []string `json:"role_name_list,omitempty"`
	IsDisabled bool     `json:"is_disabled,omitempty"`
	UserGroups []string `json:"user_group_name_list,omitempty"`
}

func convertUserToRes(user *model.User) UserDetailResV1 {
	if user.UserAuthenticationType == "" {
		user.UserAuthenticationType = model.UserAuthenticationTypeSQLE
	}
	userReq := UserDetailResV1{
		Name:       user.Name,
		Email:      user.Email,
		WeChatID:   user.WeChatID,
		LoginType:  string(user.UserAuthenticationType),
		IsAdmin:    user.Name == model.DefaultAdminUser,
		IsDisabled: user.IsDisabled(),
	}
	roleNames := make([]string, 0, len(user.Roles))
	for _, role := range user.Roles {
		roleNames = append(roleNames, role.Name)
	}
	userReq.Roles = roleNames

	userGroupNames := make([]string, len(user.UserGroups))
	for i := range user.UserGroups {
		userGroupNames[i] = user.UserGroups[i].Name
	}
	userReq.UserGroups = userGroupNames

	return userReq
}

// @Summary 获取用户信息
// @Description get user info
// @Id getUserV1
// @Tags user
// @Security ApiKeyAuth
// @Param user_name path string true "user name"
// @Success 200 {object} v1.GetUserDetailResV1
// @router /v1/users/{user_name}/ [get]
func GetUser(c echo.Context) error {
	userName := c.Param("user_name")
	s := model.GetStorage()
	user, exist, err := s.GetUserDetailByName(userName)
	if err != nil {
		return controller.JSONBaseErrorReq(c, err)
	}
	if !exist {
		return controller.JSONBaseErrorReq(c, errors.New(errors.DataNotExist, fmt.Errorf("user is not exist")))
	}
	return c.JSON(http.StatusOK, &GetUserDetailResV1{
		BaseRes: controller.NewBaseReq(nil),
		Data:    convertUserToRes(user),
	})
}

// @Summary 获取当前用户信息
// @Description get current user info
// @Id getCurrentUserV1
// @Tags user
// @Security ApiKeyAuth
// @Success 200 {object} v1.GetUserDetailResV1
// @router /v1/user [get]
func GetCurrentUser(c echo.Context) error {
	userName := controller.GetUserName(c)
	s := model.GetStorage()
	user, exist, err := s.GetUserDetailByName(userName)
	if err != nil {
		return controller.JSONBaseErrorReq(c, err)
	}
	if !exist {
		return controller.JSONBaseErrorReq(c, errors.New(errors.DataNotExist, fmt.Errorf("user is not exist")))
	}
	return c.JSON(http.StatusOK, &GetUserDetailResV1{
		BaseRes: controller.NewBaseReq(nil),
		Data:    convertUserToRes(user),
	})
}

type UpdateCurrentUserReqV1 struct {
	Email    *string `json:"email"`
	WeChatID *string `json:"wechat_id" example:"UserID"`
}

// @Summary 更新个人信息
// @Description update current user
// @Id updateCurrentUserV1
// @Tags user
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param instance body v1.UpdateCurrentUserReqV1 true "update user"
// @Success 200 {object} controller.BaseRes
// @router /v1/user [patch]
func UpdateCurrentUser(c echo.Context) error {
	req := new(UpdateUserReqV1)
	if err := controller.BindAndValidateReq(c, req); err != nil {
		return err
	}
	user, err := controller.GetCurrentUser(c)
	if err != nil {
		return controller.JSONBaseErrorReq(c, err)
	}
	s := model.GetStorage()
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.WeChatID != nil {
		user.WeChatID = *req.WeChatID
	}
	err = s.Save(user)
	if err != nil {
		return controller.JSONBaseErrorReq(c, err)
	}
	return controller.JSONBaseErrorReq(c, nil)
}

type UpdateCurrentUserPasswordReqV1 struct {
	Password    string `json:"password" valid:"required"`
	NewPassword string `json:"new_password"  valid:"required"`
}

var errLdapUserCanNotChangePassword = errors.New(errors.DataConflict, _errors.New("the password of the ldap user cannot be changed or reset, because this password is meaningless"))

// @Summary 用户修改密码
// @Description update current user's password
// @Id UpdateCurrentUserPasswordV1
// @Tags user
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param instance body v1.UpdateCurrentUserPasswordReqV1 true "update user's password"
// @Success 200 {object} controller.BaseRes
// @router /v1/user/password [put]
func UpdateCurrentUserPassword(c echo.Context) error {
	req := new(UpdateCurrentUserPasswordReqV1)
	if err := controller.BindAndValidateReq(c, req); err != nil {
		return err
	}
	user, err := controller.GetCurrentUser(c)
	if err != nil {
		return controller.JSONBaseErrorReq(c, err)
	}
	if user.UserAuthenticationType == model.UserAuthenticationTypeLDAP {
		return controller.JSONBaseErrorReq(c, errLdapUserCanNotChangePassword)
	}
	if user.Password != req.Password {
		return controller.JSONBaseErrorReq(c,
			errors.New(errors.LoginAuthFail, fmt.Errorf("password is wrong")))
	}

	s := model.GetStorage()
	err = s.UpdatePassword(user, req.NewPassword)
	if err != nil {
		return controller.JSONBaseErrorReq(c, err)
	}
	return controller.JSONBaseErrorReq(c, nil)
}

type GetUsersReqV1 struct {
	FilterUserName string `json:"filter_user_name" query:"filter_user_name"`
	FilterRoleName string `json:"filter_role_name" query:"filter_role_name"`
	PageIndex      uint32 `json:"page_index" query:"page_index" valid:"required"`
	PageSize       uint32 `json:"page_size" query:"page_size" valid:"required"`
}

type GetUsersResV1 struct {
	controller.BaseRes
	Data      []UserResV1 `json:"data"`
	TotalNums uint64      `json:"total_nums"`
}

type UserResV1 struct {
	Name       string   `json:"user_name"`
	Email      string   `json:"email"`
	WeChatID   string   `json:"wechat_id"`
	LoginType  string   `json:"login_type"`
	IsDisabled bool     `json:"is_disabled,omitempty"`
	Roles      []string `json:"role_name_list,omitempty"`
	UserGroups []string `json:"user_group_name_list,omitempty"`
}

// @Summary 获取用户信息列表
// @Description get user info list
// @Tags user
// @Id getUserListV1
// @Security ApiKeyAuth
// @Param filter_user_name query string false "filter user name"
// @Param filter_role_name query string false "filter role name"
// @Param page_index query uint32 false "page index"
// @Param page_size query uint32 false "size of per page"
// @Success 200 {object} v1.GetUsersResV1
// @router /v1/users [get]
func GetUsers(c echo.Context) error {
	req := new(GetUsersReqV1)
	if err := controller.BindAndValidateReq(c, req); err != nil {
		return err
	}
	s := model.GetStorage()

	var offset uint32
	if req.PageIndex >= 1 {
		offset = req.PageSize * (req.PageIndex - 1)
	}
	data := map[string]interface{}{
		"filter_user_name": req.FilterUserName,
		"filter_role_name": req.FilterRoleName,
		"limit":            req.PageSize,
		"offset":           offset,
	}

	users, count, err := s.GetUsersByReq(data)
	if err != nil {
		return controller.JSONBaseErrorReq(c, err)
	}

	usersReq := []UserResV1{}
	for _, user := range users {
		if user.LoginType == "" {
			user.LoginType = string(model.UserAuthenticationTypeSQLE)
		}
		userReq := UserResV1{
			Name:       user.Name,
			Email:      user.Email,
			WeChatID:   user.WeChatID.String,
			LoginType:  user.LoginType,
			Roles:      user.RoleNames,
			IsDisabled: user.IsDisabled(),
			UserGroups: user.UserGroupNames,
		}
		usersReq = append(usersReq, userReq)
	}
	return c.JSON(http.StatusOK, &GetUsersResV1{
		BaseRes:   controller.NewBaseReq(nil),
		Data:      usersReq,
		TotalNums: count,
	})
}

type UserTipResV1 struct {
	Name string `json:"user_name"`
}

type GetUserTipsResV1 struct {
	controller.BaseRes
	Data []UserTipResV1 `json:"data"`
}

// @Summary 获取用户提示列表
// @Description get user tip list
// @Tags user
// @Id getUserTipListV1
// @Security ApiKeyAuth
// @Success 200 {object} v1.GetUserTipsResV1
// @router /v1/user_tips [get]
func GetUserTips(c echo.Context) error {
	s := model.GetStorage()
	users, err := s.GetAllUserTip()
	if err != nil {
		return controller.JSONBaseErrorReq(c, err)
	}
	userTipsRes := make([]UserTipResV1, 0, len(users))

	for _, user := range users {
		userTipRes := UserTipResV1{
			Name: user.Name,
		}
		userTipsRes = append(userTipsRes, userTipRes)
	}
	return c.JSON(http.StatusOK, &GetUserTipsResV1{
		BaseRes: controller.NewBaseReq(nil),
		Data:    userTipsRes,
	})
}
