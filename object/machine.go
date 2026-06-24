package object

import (
	"github.com/casosorg/casos/util"
)

const (
	MachineStatusDeploying = "Deploying"
	MachineStatusDeployed  = "Deployed"
	MachineStatusFailed    = "Failed"
)

type Machine struct {
	Owner       string `xorm:"varchar(100) notnull pk" json:"owner"`
	Name        string `xorm:"varchar(100) notnull pk" json:"name"`
	CreatedTime string `xorm:"varchar(100)" json:"createdTime"`
	DisplayName string `xorm:"varchar(100)" json:"displayName"`

	Ip         string `xorm:"varchar(100)" json:"ip"`
	Port       int    `xorm:"int" json:"port"`
	Username   string `xorm:"varchar(100)" json:"username"`
	AuthType   string `xorm:"varchar(50)" json:"authType"`
	Password   string `xorm:"varchar(500)" json:"password"`
	PrivateKey string `xorm:"mediumtext" json:"privateKey"`

	Os          string `xorm:"varchar(100)" json:"os"`
	Status      string `xorm:"varchar(50)" json:"status"`
	Role        string `xorm:"varchar(50)" json:"role"`
	Description string `xorm:"varchar(500)" json:"description"`
}

func GetGlobalMachines() ([]*Machine, error) {
	machines := []*Machine{}
	err := ormer.Engine.Asc("owner").Desc("created_time").Find(&machines)
	if err != nil {
		return nil, err
	}
	return machines, nil
}

func GetMachines(owner string) ([]*Machine, error) {
	machines := []*Machine{}
	err := ormer.Engine.Desc("created_time").Find(&machines, &Machine{Owner: owner})
	if err != nil {
		return nil, err
	}
	return machines, nil
}

func GetMachine(id string) (*Machine, error) {
	owner, name, err := util.GetOwnerAndNameFromIdWithError(id)
	if err != nil {
		return nil, err
	}
	machine := &Machine{Owner: owner, Name: name}
	existed, err := ormer.Engine.Get(machine)
	if err != nil {
		return nil, err
	}
	if !existed {
		return nil, nil
	}
	return machine, nil
}

func UpdateMachine(id string, machine *Machine) (bool, error) {
	owner, name, err := util.GetOwnerAndNameFromIdWithError(id)
	if err != nil {
		return false, err
	}
	if m, err := GetMachine(id); err != nil {
		return false, err
	} else if m == nil {
		return false, nil
	}
	affected, err2 := ormer.Engine.Where("owner = ? AND name = ?", owner, name).AllCols().Update(machine)
	if err2 != nil {
		return false, err2
	}
	return affected != 0, nil
}

func AddMachine(machine *Machine) (bool, error) {
	affected, err := ormer.Engine.Insert(machine)
	if err != nil {
		return false, err
	}
	return affected != 0, nil
}

func DeleteMachine(machine *Machine) (bool, error) {
	affected, err := ormer.Engine.Where("owner = ? AND name = ?", machine.Owner, machine.Name).Delete(&Machine{})
	if err != nil {
		return false, err
	}
	return affected != 0, nil
}
