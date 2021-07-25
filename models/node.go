package models

import (
	"encoding/json"
	"errors"
	"math/rand"
	"net"
	"strings"
	"time"
	"log"
	"github.com/go-playground/validator/v10"
	"github.com/gravitl/netmaker/database"
)

const charset = "abcdefghijklmnopqrstuvwxyz" + "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var seededRand *rand.Rand = rand.New(
	rand.NewSource(time.Now().UnixNano()))

//node struct
type Node struct {
	ID                  string   `json:"id,omitempty" bson:"id,omitempty"`
	Address             string   `json:"address" bson:"address" validate:"omitempty,ipv4"`
	Address6            string   `json:"address6" bson:"address6" validate:"omitempty,ipv6"`
	LocalAddress        string   `json:"localaddress" bson:"localaddress" validate:"omitempty,ip"`
	Name                string   `json:"name" bson:"name" validate:"omitempty,max=12,in_charset"`
	ListenPort          int32    `json:"listenport" bson:"listenport" validate:"omitempty,numeric,min=1024,max=65535"`
	PublicKey           string   `json:"publickey" bson:"publickey" validate:"required,base64"`
	Endpoint            string   `json:"endpoint" bson:"endpoint" validate:"required,ip"`
	PostUp              string   `json:"postup" bson:"postup"`
	PostDown            string   `json:"postdown" bson:"postdown"`
	AllowedIPs          []string `json:"allowedips" bson:"allowedips"`
	PersistentKeepalive int32    `json:"persistentkeepalive" bson:"persistentkeepalive" validate:"omitempty,numeric,max=1000"`
	SaveConfig          string   `json:"saveconfig" bson:"saveconfig" validate:"checkyesorno"`
	AccessKey           string   `json:"accesskey" bson:"accesskey"`
	Interface           string   `json:"interface" bson:"interface"`
	LastModified        int64    `json:"lastmodified" bson:"lastmodified"`
	KeyUpdateTimeStamp  int64    `json:"keyupdatetimestamp" bson:"keyupdatetimestamp"`
	ExpirationDateTime  int64    `json:"expdatetime" bson:"expdatetime"`
	LastPeerUpdate      int64    `json:"lastpeerupdate" bson:"lastpeerupdate"`
	LastCheckIn         int64    `json:"lastcheckin" bson:"lastcheckin"`
	MacAddress          string   `json:"macaddress" bson:"macaddress" validate:"required,mac,macaddress_unique"`
	CheckInInterval     int32    `json:"checkininterval" bson:"checkininterval"`
	Password            string   `json:"password" bson:"password" validate:"required,min=6"`
	Network             string   `json:"network" bson:"network" validate:"network_exists"`
	IsPending           bool     `json:"ispending" bson:"ispending"`
	IsEgressGateway     bool     `json:"isegressgateway" bson:"isegressgateway"`
	IsIngressGateway    bool     `json:"isingressgateway" bson:"isingressgateway"`
	EgressGatewayRanges []string `json:"egressgatewayranges" bson:"egressgatewayranges"`
	IngressGatewayRange string   `json:"ingressgatewayrange" bson:"ingressgatewayrange"`
	PostChanges         string   `json:"postchanges" bson:"postchanges"`
	StaticIP            string   `json:"staticip" bson:"staticip"`
	StaticPubKey        string   `json:"staticpubkey" bson:"staticpubkey"`
	UDPHolePunch        string   `json:"udpholepunch" bson:"udpholepunch" validate:"checkyesorno"`
}

//TODO:
//Not sure if below two methods are necessary. May want to revisit
func (node *Node) SetLastModified() {
	node.LastModified = time.Now().Unix()
}

func (node *Node) SetLastCheckIn() {
	node.LastCheckIn = time.Now().Unix()
}

func (node *Node) SetLastPeerUpdate() {
	node.LastPeerUpdate = time.Now().Unix()
}

func (node *Node) SetID() {
	node.ID = node.MacAddress + "###" + node.Network
}

func (node *Node) SetExpirationDateTime() {
	node.ExpirationDateTime = time.Unix(33174902665, 0).Unix()
}

func (node *Node) SetDefaultName() {
	if node.Name == "" {
		nodeid := StringWithCharset(5, charset)
		nodename := "node-" + nodeid
		node.Name = nodename
	}
}

func (node *Node) GetNetwork() (Network, error) {

	var network Network
	networkData, err := database.FetchRecord(database.NETWORKS_TABLE_NAME, node.Network)
	if err != nil {
		return network, err
	}
	if err = json.Unmarshal([]byte(networkData), &network); err != nil {
		return Network{}, err
	}
	return network, nil
}

//TODO: I dont know why this exists
//This should exist on the node.go struct. I'm sure there was a reason?
func (node *Node) SetDefaults() {

	//TODO: Maybe I should make Network a part of the node struct. Then we can just query the Network object for stuff.
	parentNetwork, _ := node.GetNetwork()

	node.ExpirationDateTime = time.Unix(33174902665, 0).Unix()

	if node.ListenPort == 0 {
		node.ListenPort = parentNetwork.DefaultListenPort
	}
	if node.SaveConfig == "" {
		if parentNetwork.DefaultSaveConfig != "" {
			node.SaveConfig = parentNetwork.DefaultSaveConfig
		} else {
			node.SaveConfig = "yes"
		}
	}
	if node.Interface == "" {
		node.Interface = parentNetwork.DefaultInterface
	}
	if node.PersistentKeepalive == 0 {
		node.PersistentKeepalive = parentNetwork.DefaultKeepalive
	}
	if node.PostUp == "" {
		postup := parentNetwork.DefaultPostUp
		node.PostUp = postup
	}
	if node.StaticIP == "" {
		node.StaticIP = "no"
	}
	if node.StaticPubKey == "" {
		node.StaticPubKey = "no"
	}
	if node.UDPHolePunch == "" {
		node.UDPHolePunch = parentNetwork.DefaultUDPHolePunch
		if node.UDPHolePunch == "" {
			node.UDPHolePunch = "yes"
		}
	}
	node.CheckInInterval = parentNetwork.DefaultCheckInInterval

	node.SetLastModified()
	node.SetDefaultName()
	node.SetLastCheckIn()
	node.SetLastPeerUpdate()
	node.SetID()
	node.KeyUpdateTimeStamp = time.Now().Unix()
}

func (currentNode *Node) Update(newNode *Node) error {
	log.Println("Node SaveConfig:",newNode.SaveConfig)
	if err := newNode.Validate(true); err != nil {
		return err
	}
	newNode.SetID()
	if newNode.ID == currentNode.ID {
		if data, err := json.Marshal(newNode); err != nil {
			return err
		} else {
			newNode.SetLastModified()
			err = database.Insert(newNode.ID, string(data), database.NODES_TABLE_NAME)
			return err
		}
	}
	// copy values
	return errors.New("failed to update node " + newNode.MacAddress + ", cannot change macaddress.")
}

func StringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

//Check for valid IPv4 address
//Note: We dont handle IPv6 AT ALL!!!!! This definitely is needed at some point
//But for iteration 1, lets just stick to IPv4. Keep it simple stupid.
func IsIpv4Net(host string) bool {
	return net.ParseIP(host) != nil
}

func (node *Node) Validate(isUpdate bool) error {
	log.Println("Node SaveConfig:",node.SaveConfig)
	v := validator.New()
	_ = v.RegisterValidation("macaddress_unique", func(fl validator.FieldLevel) bool {
		if isUpdate {
			return true
		}
		isFieldUnique, _ := node.IsIDUnique()
		return isFieldUnique
	})
	_ = v.RegisterValidation("network_exists", func(fl validator.FieldLevel) bool {
		_, err := node.GetNetwork()
		return err == nil
	})
	_ = v.RegisterValidation("in_charset", func(fl validator.FieldLevel) bool {
		isgood := node.NameInNodeCharSet()
		return isgood
	})
	_ = v.RegisterValidation("checkyesorno", func(fl validator.FieldLevel) bool {
		return CheckYesOrNo(fl)
	})
	err := v.Struct(node)

	return err
}

func (node *Node) IsIDUnique() (bool, error) {
	record, err := database.FetchRecord(database.NODES_TABLE_NAME, node.ID)
	if err != nil {
		return false, err
	}
	return record == "", err
}

func (node *Node) NameInNodeCharSet() bool {

	charset := "abcdefghijklmnopqrstuvwxyz1234567890-"

	for _, char := range node.Name {
		if !strings.Contains(charset, strings.ToLower(string(char))) {
			return false
		}
	}
	return true
}

func GetAllNodes() ([]Node, error) {
	var nodes []Node

	collection, err := database.FetchRecords(database.NODES_TABLE_NAME)
	if err != nil {
		return []Node{}, err
	}

	for _, value := range collection {
		var node Node
		if err := json.Unmarshal([]byte(value), &node); err != nil {
			return []Node{}, err
		}
		// add node to our array
		nodes = append(nodes, node)
	}

	return nodes, nil
}
