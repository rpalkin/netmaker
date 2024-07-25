package logic

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/gravitl/netmaker/logger"
	"github.com/gravitl/netmaker/models"
	"github.com/gravitl/netmaker/servercfg"
)

const (
	MasterUser       = "masteradministrator"
	Forbidden_Msg    = "forbidden"
	Forbidden_Err    = models.Error(Forbidden_Msg)
	Unauthorized_Msg = "unauthorized"
	Unauthorized_Err = models.Error(Unauthorized_Msg)
)

func GetSubjectsFromURL(URL string) (rsrcType models.RsrcType, rsrcID models.RsrcID) {
	urlSplit := strings.Split(URL, "/")
	rsrcType = models.RsrcType(urlSplit[1])
	if len(urlSplit) > 1 {
		rsrcID = models.RsrcID(urlSplit[2])
	}
	return
}

func networkPermissionsCheck(username string, r *http.Request) error {
	// at this point global checks should be completed
	user, err := GetUser(username)
	if err != nil {
		return err
	}
	logger.Log(0, "NET MIDDL----> 1")
	userRole, err := GetRole(user.PlatformRoleID)
	if err != nil {
		return errors.New("access denied")
	}
	if userRole.FullAccess {
		return nil
	}
	logger.Log(0, "NET MIDDL----> 2")
	// get info from header to determine the target rsrc
	targetRsrc := r.Header.Get("TARGET_RSRC")
	targetRsrcID := r.Header.Get("TARGET_RSRC_ID")
	netID := r.Header.Get("NET_ID")
	if targetRsrc == "" {
		return errors.New("target rsrc is missing")
	}
	if netID == "" {
		return errors.New("network id is missing")
	}
	if r.Method == "" {
		r.Method = http.MethodGet
	}
	if targetRsrc == models.MetricRsrc.String() {
		return nil
	}

	// check if user has scope for target resource
	// TODO - differentitate between global scope and network scope apis
	netRoles := user.NetworkRoles[models.NetworkID(netID)]
	for netRoleID := range netRoles {
		err = checkNetworkAccessPermissions(netRoleID, username, r.Method, targetRsrc, targetRsrcID)
		if err == nil {
			return nil
		}
	}
	for groupID := range user.UserGroups {
		userG, err := GetUserGroup(groupID)
		if err == nil {
			netRoles := userG.NetworkRoles[models.NetworkID(netID)]
			for netRoleID := range netRoles {
				err = checkNetworkAccessPermissions(netRoleID, username, r.Method, targetRsrc, targetRsrcID)
				if err == nil {
					return nil
				}
			}
		}
	}

	return errors.New("access denied")
}

func checkNetworkAccessPermissions(netRoleID models.UserRole, username, reqScope, targetRsrc, targetRsrcID string) error {
	networkPermissionScope, err := GetRole(netRoleID)
	if err != nil {
		return err
	}
	logger.Log(0, "NET MIDDL----> 3", string(netRoleID))
	if networkPermissionScope.FullAccess {
		return nil
	}
	rsrcPermissionScope, ok := networkPermissionScope.NetworkLevelAccess[models.RsrcType(targetRsrc)]
	if targetRsrc == models.HostRsrc.String() && !ok {
		rsrcPermissionScope, ok = networkPermissionScope.NetworkLevelAccess[models.RemoteAccessGwRsrc]
	}
	if !ok {
		return errors.New("access denied")
	}
	logger.Log(0, "NET MIDDL----> 4", string(netRoleID))
	if allRsrcsTypePermissionScope, ok := rsrcPermissionScope[models.RsrcID(fmt.Sprintf("all_%s", targetRsrc))]; ok {
		// handle extclient apis here
		if models.RsrcType(targetRsrc) == models.ExtClientsRsrc && allRsrcsTypePermissionScope.SelfOnly && targetRsrcID != "" {
			extclient, err := GetExtClient(targetRsrcID, networkPermissionScope.NetworkID)
			if err != nil {
				return err
			}
			if !IsUserAllowedAccessToExtClient(username, extclient) {
				return errors.New("access denied")
			}
		}
		err = checkPermissionScopeWithReqMethod(allRsrcsTypePermissionScope, reqScope)
		if err == nil {
			return nil
		}

	}
	if targetRsrc == models.HostRsrc.String() {
		if allRsrcsTypePermissionScope, ok := rsrcPermissionScope[models.RsrcID(fmt.Sprintf("all_%s", models.RemoteAccessGwRsrc))]; ok {
			err = checkPermissionScopeWithReqMethod(allRsrcsTypePermissionScope, reqScope)
			if err == nil {
				return nil
			}
		}
	}
	logger.Log(0, "NET MIDDL----> 5", string(netRoleID))
	if targetRsrcID == "" {
		return errors.New("target rsrc id is empty")
	}
	if scope, ok := rsrcPermissionScope[models.RsrcID(targetRsrcID)]; ok {
		err = checkPermissionScopeWithReqMethod(scope, reqScope)
		if err == nil {
			return nil
		}
	}
	logger.Log(0, "NET MIDDL----> 6", string(netRoleID))
	return errors.New("access denied")
}

func globalPermissionsCheck(username string, r *http.Request) error {
	user, err := GetUser(username)
	if err != nil {
		return err
	}
	userRole, err := GetRole(user.PlatformRoleID)
	if err != nil {
		return errors.New("access denied")
	}
	if userRole.FullAccess {
		return nil
	}
	targetRsrc := r.Header.Get("TARGET_RSRC")
	targetRsrcID := r.Header.Get("TARGET_RSRC_ID")
	if targetRsrc == "" {
		return errors.New("target rsrc is missing")
	}
	if r.Method == "" {
		r.Method = http.MethodGet
	}
	if targetRsrc == models.MetricRsrc.String() {
		return nil
	}
	if targetRsrc == models.HostRsrc.String() && r.Method == http.MethodGet && targetRsrcID == "" {
		return nil
	}
	if targetRsrc == models.UserRsrc.String() && username == targetRsrcID && (r.Method != http.MethodDelete) {
		return nil
	}
	rsrcPermissionScope, ok := userRole.GlobalLevelAccess[models.RsrcType(targetRsrc)]
	if !ok {
		return fmt.Errorf("access denied to %s rsrc", targetRsrc)
	}
	if allRsrcsTypePermissionScope, ok := rsrcPermissionScope[models.RsrcID(fmt.Sprintf("all_%s", targetRsrc))]; ok {
		return checkPermissionScopeWithReqMethod(allRsrcsTypePermissionScope, r.Method)

	}
	if targetRsrcID == "" {
		return errors.New("target rsrc id is missing")
	}
	if scope, ok := rsrcPermissionScope[models.RsrcID(targetRsrcID)]; ok {
		return checkPermissionScopeWithReqMethod(scope, r.Method)
	}
	return errors.New("access denied")
}

func checkPermissionScopeWithReqMethod(scope models.RsrcPermissionScope, reqmethod string) error {
	if reqmethod == http.MethodGet && scope.Read {
		return nil
	}
	if (reqmethod == http.MethodPatch || reqmethod == http.MethodPut) && scope.Update {
		return nil
	}
	if reqmethod == http.MethodDelete && scope.Delete {
		return nil
	}
	if reqmethod == http.MethodPost && scope.Create {
		return nil
	}
	return errors.New("operation not permitted")
}

// SecurityCheck - Check if user has appropriate permissions
func SecurityCheck(reqAdmin bool, next http.Handler) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("ismaster", "no")
		logger.Log(0, "next", r.URL.String())
		isGlobalAccesss := r.Header.Get("IS_GLOBAL_ACCESS") == "yes"
		bearerToken := r.Header.Get("Authorization")
		username, err := GetUserNameFromToken(bearerToken)
		if err != nil {
			logger.Log(0, "next 1", r.URL.String(), err.Error())
			ReturnErrorResponse(w, r, FormatError(err, err.Error()))
			return
		}
		// detect masteradmin
		if username == MasterUser {
			r.Header.Set("ismaster", "yes")
		} else {
			if isGlobalAccesss {
				err = globalPermissionsCheck(username, r)
			} else {
				err = networkPermissionsCheck(username, r)
			}
		}
		w.Header().Set("TARGET_RSRC", r.Header.Get("TARGET_RSRC"))
		w.Header().Set("TARGET_RSRC_ID", r.Header.Get("TARGET_RSRC_ID"))
		w.Header().Set("RSRC_TYPE", r.Header.Get("RSRC_TYPE"))
		w.Header().Set("IS_GLOBAL_ACCESS", r.Header.Get("IS_GLOBAL_ACCESS"))
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if err != nil {
			w.Header().Set("ACCESS_PERM", err.Error())
			ReturnErrorResponse(w, r, FormatError(err, "forbidden"))
			return
		}
		r.Header.Set("user", username)
		next.ServeHTTP(w, r)
	}
}

// UserPermissions - checks token stuff
func UserPermissions(reqAdmin bool, token string) (string, error) {
	var tokenSplit = strings.Split(token, " ")
	var authToken = ""

	if len(tokenSplit) < 2 {
		return "", Unauthorized_Err
	} else {
		authToken = tokenSplit[1]
	}
	//all endpoints here require master so not as complicated
	if authenticateMaster(authToken) {
		// TODO log in as an actual admin user
		return MasterUser, nil
	}
	username, issuperadmin, isadmin, err := VerifyUserToken(authToken)
	if err != nil {
		return username, Unauthorized_Err
	}
	if reqAdmin && !(issuperadmin || isadmin) {
		return username, Forbidden_Err
	}

	return username, nil
}

// Consider a more secure way of setting master key
func authenticateMaster(tokenString string) bool {
	return tokenString == servercfg.GetMasterKey() && servercfg.GetMasterKey() != ""
}

func ContinueIfUserMatch(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var errorResponse = models.ErrorResponse{
			Code: http.StatusForbidden, Message: Forbidden_Msg,
		}
		var params = mux.Vars(r)
		var requestedUser = params["username"]
		if requestedUser != r.Header.Get("user") {
			logger.Log(0, "next 2", r.URL.String(), errorResponse.Message)
			ReturnErrorResponse(w, r, errorResponse)
			return
		}
		next.ServeHTTP(w, r)
	}
}
