package api

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/actiontech/sqle/sqle/api/cloudbeaver_wrapper"
	"github.com/actiontech/sqle/sqle/api/controller"
	v1 "github.com/actiontech/sqle/sqle/api/controller/v1"
	v2 "github.com/actiontech/sqle/sqle/api/controller/v2"
	sqleMiddleware "github.com/actiontech/sqle/sqle/api/middleware"
	"github.com/actiontech/sqle/sqle/config"
	_ "github.com/actiontech/sqle/sqle/docs"
	"github.com/actiontech/sqle/sqle/errors"
	"github.com/actiontech/sqle/sqle/log"
	"github.com/actiontech/sqle/sqle/model"
	"github.com/actiontech/sqle/sqle/utils"

	"github.com/facebookgo/grace/gracenet"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	echoSwagger "github.com/swaggo/echo-swagger"
)

const (
	apiV1 = "v1"
	apiV2 = "v2"
)

// @title Sqle API Docs
// @version 1.0
// @description This is a sample server for dev.
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
// @BasePath /
func StartApi(net *gracenet.Net, exitChan chan struct{}, config config.SqleConfig) {
	defer close(exitChan)

	e := echo.New()
	output := log.NewRotateFile(config.LogPath, "/api.log", 1024 /*1GB*/)
	defer output.Close()

	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Output: output,
	}))
	e.HideBanner = true
	e.HidePort = true

	// custom handler http error
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if _, ok := err.(*errors.CodeError); ok {
			if err = controller.JSONBaseErrorReq(c, err); err != nil {
				log.NewEntry().Error("send json error response failed, error:", err)
			}
		} else {
			e.DefaultHTTPErrorHandler(err, c)
		}
	}

	e.GET("/swagger/*", echoSwagger.WrapHandler)

	e.POST("/v1/login", v1.Login, cloudbeaver_wrapper.TriggerLogin())

	// the operation of obtaining the basic information of the platform should be for all users, not the users who log in to the platform
	e.GET("/v1/basic_info", v1.GetSQLEInfo)

	// oauth2 interface does not require login authentication
	e.GET("/v1/configurations/oauth2/tips", v1.GetOauth2Tips)
	e.GET("/v1/oauth2/link", v1.Oauth2Link)
	e.GET("/v1/oauth2/callback", v1.Oauth2Callback)
	e.POST("/v1/oauth2/user/bind", v1.BindOauth2User)

	v1Router := e.Group(apiV1)
	v1Router.Use(sqleMiddleware.JWTTokenAdapter(), middleware.JWT(utils.JWTSecretKey), sqleMiddleware.VerifyUserIsDisabled(), sqleMiddleware.LicenseAdapter())
	v2Router := e.Group(apiV2)
	v2Router.Use(sqleMiddleware.JWTTokenAdapter(), middleware.JWT(utils.JWTSecretKey), sqleMiddleware.VerifyUserIsDisabled(), sqleMiddleware.LicenseAdapter())

	// v1 admin api, just admin user can access.
	{
		// user
		v1Router.GET("/users", v1.GetUsers, AdminUserAllowed())
		v1Router.POST("/users", v1.CreateUser, AdminUserAllowed())
		v1Router.GET("/users/:user_name/", v1.GetUser, AdminUserAllowed())
		v1Router.PATCH("/users/:user_name/", v1.UpdateUser, AdminUserAllowed())
		v1Router.DELETE("/users/:user_name/", v1.DeleteUser, AdminUserAllowed())
		v1Router.PATCH("/users/:user_name/password", v1.UpdateOtherUserPassword, AdminUserAllowed())

		// user_group
		v1Router.POST("/user_groups", v1.CreateUserGroup, AdminUserAllowed())
		v1Router.GET("/user_groups", v1.GetUserGroups, AdminUserAllowed())
		v1Router.DELETE("/user_groups/:user_group_name/", v1.DeleteUserGroup, AdminUserAllowed())
		v1Router.PATCH("/user_groups/:user_group_name/", v1.UpdateUserGroup, AdminUserAllowed())
		v1Router.GET("/user_group_tips", v1.GetUserGroupTips, AdminUserAllowed())

		// role
		v1Router.GET("/roles", DeprecatedBy(apiV2), AdminUserAllowed())
		v2Router.GET("/roles", v2.GetRoles, AdminUserAllowed())
		v1Router.GET("/role_tips", v1.GetRoleTips, AdminUserAllowed())
		v1Router.POST("/roles", DeprecatedBy(apiV2), AdminUserAllowed())
		v2Router.POST("/roles", v2.CreateRole, AdminUserAllowed())
		v1Router.PATCH("/roles/:role_name/", DeprecatedBy(apiV2), AdminUserAllowed())
		v2Router.PATCH("/roles/:role_name/", v2.UpdateRole, AdminUserAllowed())
		v1Router.DELETE("/roles/:role_name/", v1.DeleteRole, AdminUserAllowed())

		// instance
		v1Router.POST("/instances", v1.CreateInstance, AdminUserAllowed())
		v1Router.GET("/instance_additional_metas", v1.GetInstanceAdditionalMetas, AdminUserAllowed())
		v1Router.DELETE("/instances/:instance_name/", v1.DeleteInstance, AdminUserAllowed())
		v1Router.PATCH("/instances/:instance_name/", v1.UpdateInstance, AdminUserAllowed())

		// rule template
		v1Router.POST("/rule_templates", v1.CreateRuleTemplate, AdminUserAllowed())
		v1Router.POST("/rule_templates/:rule_template_name/clone", v1.CloneRuleTemplate, AdminUserAllowed())
		v1Router.PATCH("/rule_templates/:rule_template_name/", v1.UpdateRuleTemplate, AdminUserAllowed())
		v1Router.DELETE("/rule_templates/:rule_template_name/", v1.DeleteRuleTemplate, AdminUserAllowed())

		// workflow template
		v1Router.GET("/workflow_templates", v1.GetWorkflowTemplates, AdminUserAllowed())
		v1Router.POST("/workflow_templates", v1.CreateWorkflowTemplate, AdminUserAllowed())
		v1Router.GET("/workflow_templates/:workflow_template_name/", v1.GetWorkflowTemplate, AdminUserAllowed())
		v1Router.PATCH("/workflow_templates/:workflow_template_name/", v1.UpdateWorkflowTemplate, AdminUserAllowed())
		v1Router.DELETE("/workflow_templates/:workflow_template_name/", v1.DeleteWorkflowTemplate, AdminUserAllowed())
		v1Router.GET("/workflow_template_tips", v1.GetWorkflowTemplateTips, AdminUserAllowed())

		// workflow
		v1Router.POST("/workflows/cancel", v1.BatchCancelWorkflows, AdminUserAllowed())

		// audit whitelist
		v1Router.GET("/audit_whitelist", v1.GetSqlWhitelist, AdminUserAllowed())
		v1Router.POST("/audit_whitelist", v1.CreateAuditWhitelist, AdminUserAllowed())
		v1Router.PATCH("/audit_whitelist/:audit_whitelist_id/", v1.UpdateAuditWhitelistById, AdminUserAllowed())
		v1Router.DELETE("/audit_whitelist/:audit_whitelist_id/", v1.DeleteAuditWhitelistById, AdminUserAllowed())

		// configurations
		v1Router.GET("/configurations/ldap", v1.GetLDAPConfiguration, AdminUserAllowed())
		v1Router.PATCH("/configurations/ldap", v1.UpdateLDAPConfiguration, AdminUserAllowed())
		v1Router.GET("/configurations/smtp", v1.GetSMTPConfiguration, AdminUserAllowed())
		v1Router.POST("/configurations/smtp/test", v1.TestSMTPConfigurationV1, AdminUserAllowed())
		v1Router.PATCH("/configurations/smtp", v1.UpdateSMTPConfiguration, AdminUserAllowed())
		v1Router.GET("/configurations/wechat", v1.GetWeChatConfiguration, AdminUserAllowed())
		v1Router.PATCH("/configurations/wechat", v1.UpdateWeChatConfigurationV1, AdminUserAllowed())
		v1Router.POST("/configurations/wechat/test", v1.TestWeChatConfigurationV1, AdminUserAllowed())
		v1Router.GET("/configurations/system_variables", v1.GetSystemVariables, AdminUserAllowed())
		v1Router.PATCH("/configurations/system_variables", v1.UpdateSystemVariables, AdminUserAllowed())
		v1Router.GET("/configurations/license", v1.GetLicense, AdminUserAllowed())
		v1Router.POST("/configurations/license", v1.SetLicense, AdminUserAllowed())
		v1Router.GET("/configurations/license/info", v1.GetSQLELicenseInfo, AdminUserAllowed())
		v1Router.POST("/configurations/license/check", v1.CheckLicense, AdminUserAllowed())
		v1Router.GET("/configurations/oauth2", v1.GetOauth2Configuration, AdminUserAllowed())
		v1Router.PATCH("/configurations/oauth2", v1.UpdateOauth2Configuration, AdminUserAllowed())

		// statistic
		v1Router.GET("/statistic/instances/type_percent", v1.GetInstancesTypePercentV1, AdminUserAllowed())
		v1Router.GET("/statistic/instances/sql_average_execution_time", v1.GetSqlAverageExecutionTimeV1, AdminUserAllowed())
		v1Router.GET("/statistic/instances/sql_execution_fail_percent", v1.GetSqlExecutionFailPercentV1, AdminUserAllowed())
		v1Router.GET("/statistic/license/usage", v1.GetLicenseUsageV1, AdminUserAllowed())
		v1Router.GET("/statistic/workflows/rejected_percent_group_by_creator", v1.GetWorkflowRejectedPercentGroupByCreatorV1, AdminUserAllowed())
		//v1Router.GET("/statistic/workflows/rejected_percent_group_by_instance", v1.GetWorkflowRejectedPercentGroupByInstanceV1, AdminUserAllowed())
		v1Router.GET("/statistic/workflows/counts", v1.GetWorkflowCountsV1, AdminUserAllowed())
		v1Router.GET("/statistic/workflows/duration_of_waiting_for_audit", v1.GetWorkflowDurationOfWaitingForAuditV1, AdminUserAllowed())
		//v1Router.GET("/statistic/workflows/duration_of_waiting_for_execution", v1.GetWorkflowDurationOfWaitingForExecutionV1, AdminUserAllowed())
		//v1Router.GET("/statistic/workflows/pass_percent", v1.GetWorkflowPassPercentV1, AdminUserAllowed())
		v1Router.GET("/statistic/workflows/audit_pass_percent", v1.GetWorkflowAuditPassPercentV1, AdminUserAllowed())
		v1Router.GET("/statistic/workflows/each_day_counts", v1.GetWorkflowCreatedCountsEachDayV1, AdminUserAllowed())
		v1Router.GET("/statistic/workflows/status_count", v1.GetWorkflowStatusCountV1, AdminUserAllowed())
		v1Router.GET("/statistic/workflows/instance_type_percent", v1.GetWorkflowPercentCountedByInstanceTypeV1, AdminUserAllowed())
	}

	// user
	v1Router.GET("/user", v1.GetCurrentUser)
	v1Router.PATCH("/user", v1.UpdateCurrentUser)
	v1Router.GET("/user_tips", v1.GetUserTips)
	v1Router.PUT("/user/password", v1.UpdateCurrentUserPassword)

	// operations
	v1Router.GET("/operations", v1.GetOperations)

	// instance
	v1Router.GET("/instances", v1.GetInstances)
	v1Router.GET("/instances/:instance_name/", v1.GetInstance)
	v1Router.GET("/instances/:instance_name/connection", v1.CheckInstanceIsConnectableByName)
	v1Router.POST("/instance_connection", v1.CheckInstanceIsConnectable)
	v1Router.POST("/instances/connections", v1.BatchCheckInstanceConnections)
	v1Router.GET("/instances/:instance_name/schemas", v1.GetInstanceSchemas)
	v1Router.GET("/instance_tips", v1.GetInstanceTips)
	v1Router.GET("/instances/:instance_name/rules", v1.GetInstanceRules)
	v1Router.GET("/instances/:instance_name/workflow_template", v1.GetInstanceWorkflowTemplate)
	v1Router.GET("/instances/:instance_name/schemas/:schema_name/tables", v1.ListTableBySchema)
	v1Router.GET("/instances/:instance_name/schemas/:schema_name/tables/:table_name/metadata", v1.GetTableMetadata)

	// rule template
	v1Router.GET("/rule_templates", v1.GetRuleTemplates)
	v1Router.GET("/rule_template_tips", v1.GetRuleTemplateTips)
	v1Router.GET("/rule_templates/:rule_template_name/", v1.GetRuleTemplate)

	//rule
	v1Router.GET("/rules", v1.GetRules)

	// workflow
	v1Router.POST("/workflows", DeprecatedBy(apiV2))
	v2Router.POST("/workflows", v2.CreateWorkflowV2)
	v1Router.GET("/workflows/:workflow_id/", DeprecatedBy(apiV2))
	v2Router.GET("/workflows/:workflow_id/", v2.GetWorkflowV2)
	v1Router.GET("/workflows", DeprecatedBy(apiV2))
	v2Router.GET("/workflows", v2.GetWorkflowsV2)
	v1Router.POST("/workflows/:workflow_id/steps/:workflow_step_id/approve", v1.ApproveWorkflow)
	v1Router.POST("/workflows/:workflow_id/steps/:workflow_step_id/reject", v1.RejectWorkflow)
	v1Router.POST("/workflows/:workflow_id/cancel", v1.CancelWorkflow)
	v1Router.PATCH("/workflows/:workflow_id/", DeprecatedBy(apiV2))
	v2Router.PATCH("/workflows/:workflow_id/", v2.UpdateWorkflowV2)
	v1Router.PUT("/workflows/:workflow_id/schedule", DeprecatedBy(apiV2))
	v2Router.PUT("/workflows/:workflow_id/tasks/:task_id/schedule", v2.UpdateWorkflowScheduleV2)
	v1Router.POST("/workflows/:workflow_id/task/execute", DeprecatedBy(apiV2))
	v2Router.POST("/workflows/:workflow_id/tasks/execute", v2.ExecuteTasksOnWorkflow)
	v1Router.POST("/workflows/:workflow_id/tasks/:task_id/execute", v1.ExecuteOneTaskOnWorkflowV1)
	v1Router.GET("/workflows/:workflow_id/tasks", v1.GetSummaryOfWorkflowTasksV1)

	// task
	v1Router.POST("/tasks/audits", v1.CreateAndAuditTask)
	v1Router.GET("/tasks/audits/:task_id/", v1.GetTask)
	v1Router.GET("/tasks/audits/:task_id/sqls", v1.GetTaskSQLs)
	v1Router.GET("/tasks/audits/:task_id/sql_report", v1.DownloadTaskSQLReportFile)
	v1Router.GET("/tasks/audits/:task_id/sql_file", v1.DownloadTaskSQLFile)
	v1Router.GET("/tasks/audits/:task_id/sql_content", v1.GetAuditTaskSQLContent)
	v1Router.PATCH("/tasks/audits/:task_id/sqls/:number", v1.UpdateAuditTaskSQLs)
	v1Router.GET("/tasks/audits/:task_id/sqls/:number/analysis", v1.GetTaskAnalysisData)
	v1Router.POST("/task_groups", v1.CreateAuditTasksGroupV1)
	v1Router.POST("/task_groups/audit", v1.AuditTaskGroupV1)

	// dashboard
	v1Router.GET("/dashboard", v1.Dashboard)

	// configurations
	v1Router.GET("/configurations/drivers", v1.GetDrivers)
	v1Router.GET("/configurations/sql_query", v1.GetSQLQueryConfiguration)

	// audit plan
	v1Router.POST("/audit_plans", v1.CreateAuditPlan)
	v1Router.DELETE("/audit_plans/:audit_plan_name/", v1.DeleteAuditPlan)
	v1Router.PATCH("/audit_plans/:audit_plan_name/", v1.UpdateAuditPlan)
	v1Router.GET("/audit_plans/:audit_plan_name/", v1.GetAuditPlan)
	v1Router.GET("/audit_plans", v1.GetAuditPlans)
	v1Router.GET("/audit_plans/:audit_plan_name/reports", v1.GetAuditPlanReports)
	v1Router.GET("/audit_plans/:audit_plan_name/reports/:audit_plan_report_id/", v1.GetAuditPlanReport)
	// deprecated
	v1Router.GET("/audit_plans/:audit_plan_name/report/:audit_plan_report_id/", DeprecatedBy(apiV2))
	v2Router.GET("/audit_plans/:audit_plan_name/report/:audit_plan_report_id/", v2.GetAuditPlanReportSQLs)
	v2Router.GET("/audit_plans/:audit_plan_name/reports/:audit_plan_report_id/sqls", v2.GetAuditPlanReportSQLsV2)
	// deprecated
	v1Router.GET("/audit_plans/:audit_plan_name/sqls", DeprecatedBy(apiV2))
	v2Router.GET("/audit_plans/:audit_plan_name/sqls", v2.GetAuditPlanSQLs)

	v1Router.POST("/audit_plans/:audit_plan_name/sqls/full", v1.FullSyncAuditPlanSQLs, sqleMiddleware.ScannerVerifier())
	v1Router.POST("/audit_plans/:audit_plan_name/sqls/partial", v1.PartialSyncAuditPlanSQLs, sqleMiddleware.ScannerVerifier())
	v1Router.POST("/audit_plans/:audit_plan_name/trigger", v1.TriggerAuditPlan)
	v1Router.GET("/audit_plan_metas", v1.GetAuditPlanMetas)
	v1Router.GET("/audit_plan_types", v1.GetAuditPlanTypes)
	v1Router.PATCH("/audit_plans/:audit_plan_name/notify_config", v1.UpdateAuditPlanNotifyConfig)
	v1Router.GET("/audit_plans/:audit_plan_name/notify_config", v1.GetAuditPlanNotifyConfig)
	v1Router.GET("/audit_plans/:audit_plan_name/notify_config/test", v1.TestAuditPlanNotifyConfig)
	v1Router.GET("/audit_plans/reports/:audit_plan_report_id/sqls/:number/analysis", v1.GetAuditPlanAnalysisData)

	// sql query
	cloudbeaver_wrapper.StartApp(e)
	//v1Router.POST("/sql_query/prepare/:instance_name/", v1.PrepareSQLQuery)
	//v1Router.GET("/sql_query/history/:instance_name/", v1.GetSQLQueryHistory)
	//v1Router.GET("/sql_query/results/:query_id/", v1.GetSQLResult)
	//v1Router.POST("/sql_query/explain/:instance_name/", v1.GetSQLExplain)

	// sql audit
	v1Router.POST("/sql_audit", v1.DirectAudit)

	// UI
	e.File("/", "ui/index.html")
	e.Static("/static", "ui/static")
	e.File("/favicon.png", "ui/favicon.png")
	e.GET("/*", func(c echo.Context) error {
		return c.File("ui/index.html")
	})

	address := fmt.Sprintf(":%v", config.SqleServerPort)
	log.Logger().Infof("starting http server on %s", address)

	// start http server
	l, err := net.Listen("tcp4", address)
	if err != nil {
		log.Logger().Fatal(err)
		return
	}
	if config.EnableHttps {
		// Usually, it is easier to create an tls server using echo#StartTLS;
		// but I need create a graceful listener.
		if config.CertFilePath == "" || config.KeyFilePath == "" {
			log.Logger().Fatal("invalid tls configuration")
			return
		}
		tlsConfig := new(tls.Config)
		tlsConfig.Certificates = make([]tls.Certificate, 1)
		tlsConfig.Certificates[0], err = tls.LoadX509KeyPair(config.CertFilePath, config.KeyFilePath)
		if err != nil {
			log.Logger().Fatal("load x509 key pair failed, error:", err)
			return
		}
		e.TLSServer.TLSConfig = tlsConfig
		e.TLSListener = tls.NewListener(l, tlsConfig)

		log.Logger().Fatal(e.StartServer(e.TLSServer))
	} else {
		e.Listener = l
		log.Logger().Fatal(e.Start(""))
	}
}

// AdminUserAllowed is a `echo` middleware, only allow admin user to access next.
func AdminUserAllowed() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if controller.GetUserName(c) == model.DefaultAdminUser {
				return next(c)
			}
			return echo.NewHTTPError(http.StatusForbidden)
		}
	}
}

// DeprecatedBy is a controller used to mark deprecated and used to replace the original controller.
func DeprecatedBy(version string) func(echo.Context) error {
	return func(ctx echo.Context) error {
		return echo.NewHTTPError(http.StatusForbidden, fmt.Sprintf(
			"the API has been deprecated, please using the %s version", version))
	}
}
