package dissect

func operatorRole(operator Operator) (TeamRole, bool) {
	role, ok := _operatorRoles[operator]
	return role, ok
}
