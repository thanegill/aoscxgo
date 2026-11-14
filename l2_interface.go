package aoscxgo

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
)

type L2Interface struct {

	// Connection properties.
	Interface        Interface              `json:"interface"`
	Description      string                 `json:"description"`
	VlanMode         string                 `json:"vlan_mode"`
	VlanIds          []interface{}          `json:"vlan_ids"`
	VlanTag          int                    `json:"vlan_tag"`
	TrunkAllowedAll  bool                   `json:"trunk_allowed_all"`
	NativeVlanTag    bool                   `json:"native_vlan_tag"`
	InterfaceDetails map[string]interface{} `json:"details"`
	materialized     bool                   `json:"materialized"`
}

// Create sets the port's VLANs exactly as Update does, since a physical port
// always exists; a name that is no port fails its PATCH.
func (i *L2Interface) Create(c *Client) error {
	return i.Update(c)
}

// Update PATCHes vlan_mode, vlan_tag and vlan_trunks in one request, and no
// other attribute: user_config holds the port's MTU and admin state, and a
// PUT would replace every writable attribute. Every VLAN it names must
// already exist, since the switch would drop a missing one from vlan_trunks
// and an empty vlan_trunks allows every VLAN.
func (i *L2Interface) Update(c *Client) error {
	if !checkName(i.Interface.Name) {
		return &RequestError{
			StatusCode: "Invalid Required Value: Name",
			Err:        errors.New("Update Error"),
		}
	}

	patchMap := map[string]interface{}{}

	switch i.VlanMode {
	case "access":
		tag, err := vlanReferences(c, []interface{}{i.VlanTag})
		if err != nil {
			return err
		}
		patchMap["vlan_mode"] = "access"
		patchMap["vlan_tag"] = tag
		patchMap["vlan_trunks"] = map[string]interface{}{}
	case "trunk":
		tag, err := vlanReferences(c, []interface{}{i.VlanTag})
		if err != nil {
			return err
		}
		trunks := map[string]interface{}{}
		if i.TrunkAllowedAll && len(i.VlanIds) != 0 {
			return &RequestError{
				StatusCode: "a trunk sets trunk_allowed_all or lists vlan_ids, not both",
				Err:        errors.New("Update Error"),
			}
		}
		if !i.TrunkAllowedAll {
			if len(i.VlanIds) == 0 {
				return &RequestError{
					StatusCode: "a trunk lists its allowed VLANs in vlan_ids, or sets trunk_allowed_all",
					Err:        errors.New("Update Error"),
				}
			}
			// The switch forwards the native VLAN only when it is allowed too.
			if !containsVlan(i.VlanIds, i.VlanTag) {
				return &RequestError{
					StatusCode: "the native VLAN " + strconv.Itoa(i.VlanTag) + " is not among the trunk's allowed VLANs",
					Err:        errors.New("Update Error"),
				}
			}
			trunks, err = vlanReferences(c, i.VlanIds)
			if err != nil {
				return err
			}
		}
		if i.NativeVlanTag {
			patchMap["vlan_mode"] = "native-tagged"
		} else {
			patchMap["vlan_mode"] = "native-untagged"
		}
		patchMap["vlan_tag"] = tag
		patchMap["vlan_trunks"] = trunks
	default:
		return &RequestError{
			StatusCode: "Invalid Required Value: VlanMode - valid options are 'access' or 'trunk' received: " + i.VlanMode,
			Err:        errors.New("Update Error"),
		}
	}

	patchBody, _ := json.Marshal(patchMap)
	address := "https://" + c.Hostname + "/rest/" + c.Version + "/system/interfaces/" + url.PathEscape(i.Interface.Name)
	res := patch(c, address, bytes.NewBuffer(patchBody))
	if res.Status != "204 No Content" {
		return &RequestError{
			StatusCode: string(patchBody) + " PATCH status code = " + res.Status,
			Err:        errors.New("Update Error"),
		}
	}

	i.materialized = true
	return nil
}

// vlanReferences maps each VLAN id to its URI, as vlan_tag and vlan_trunks
// name them, failing on any VLAN the switch does not have.
func vlanReferences(c *Client, ids []interface{}) (map[string]interface{}, error) {
	references := map[string]interface{}{}
	for _, id := range ids {
		vlan := Vlan{VlanId: id.(int)}
		if vlan.VlanId < 1 || vlan.VlanId > 4094 {
			return nil, &RequestError{
				StatusCode: "VLAN " + strconv.Itoa(vlan.VlanId) + " is not a VLAN id",
				Err:        errors.New("Update Error"),
			}
		}
		if err := vlan.Get(c); err != nil {
			return nil, &RequestError{
				StatusCode: "VLAN " + strconv.Itoa(vlan.VlanId) + " could not be read from the switch: " + err.(*RequestError).StatusCode,
				Err:        errors.New("Update Error"),
			}
		}
		references[strconv.Itoa(vlan.VlanId)] = vlan.GetURI()
	}
	return references, nil
}

func containsVlan(ids []interface{}, id int) bool {
	for _, item := range ids {
		if item.(int) == id {
			return true
		}
	}
	return false
}

// Delete performs PUT to remove/default L2Interface configuration from the given Client object.
func (i *L2Interface) Delete(c *Client) error {
	base_uri := "system/interfaces"
	if i.Interface.Name == "" {
		return &RequestError{
			StatusCode: "Missing Interface unable to delete L2Interface",
			Err:        errors.New("Delete Error"),
		}
	}
	int_str := url.PathEscape(i.Interface.Name)

	putMap := map[string]interface{}{}

	putBody, _ := json.Marshal(putMap)

	json_body := bytes.NewBuffer(putBody)

	url := "https://" + c.Hostname + "/rest/" + c.Version + "/" + base_uri + "/" + int_str

	//need logic for handling interfaces between platforms

	res := put(c, url, json_body)

	if res.Status != "204 No Content" && res.Status != "200 OK" {
		return &RequestError{
			StatusCode: res.Status,
			Err:        errors.New("Delete Error"),
		}
	}

	return nil
}

// Get performs GET to retrieve L2Interface configuration from the given Client object.
func (i *L2Interface) Get(c *Client) error {
	base_uri := "system/interfaces"
	if i.Interface.Name == "" {
		return &RequestError{
			StatusCode: "Missing Interface unable to retrieve L2Interface",
			Err:        errors.New("Get Error"),
		}
	}
	int_str := url.PathEscape(i.Interface.Name)

	url := "https://" + c.Hostname + "/rest/" + c.Version + "/" + base_uri + "/" + int_str + "?selector=writable"

	res, body := get(c, url)

	if res.Status != "200 OK" {
		i.materialized = false
		return &RequestError{
			StatusCode: res.Status,
			Err:        errors.New("Get Error"),
		}
	}

	if i.Interface.InterfaceDetails == nil {
		i.Interface.InterfaceDetails = map[string]interface{}{}
	}

	for key, value := range body {
		i.Interface.InterfaceDetails[key] = value
		if key == "description" && value != nil {
			i.Description = value.(string)
			i.Interface.Description = value.(string)
		}

		if key == "vlan_mode" && value != nil {
			i.VlanMode = value.(string)
			if i.VlanMode == "native-tagged" {
				i.NativeVlanTag = true
			} else {
				i.NativeVlanTag = false
			}
		}

		if key == "admin" && value != nil {
			i.Interface.AdminState = value.(string)
		}

		if key == "vlan_tag" {
			// convert json to value singular
			// "vlan_tag": {
			// 	"42": "/rest/v10.09/system/vlans/42"
			//   },
			tags, _ := value.(map[string]interface{})
			for key, _ := range tags {
				vlan_int, _ := strconv.Atoi(key)
				i.VlanTag = vlan_int
			}

		}

		if key == "vlan_trunks" {
			trunks, _ := value.(map[string]interface{})
			i.VlanIds = []interface{}{}
			for key, _ := range trunks {
				vlan_int, _ := strconv.Atoi(key)
				i.VlanIds = append(i.VlanIds, vlan_int)
			}
			i.TrunkAllowedAll = len(trunks) == 0
		}

	}

	i.materialized = true

	return nil
}

// GetStatus returns True if L2Interface exists on Client object or False if not.
func (i *L2Interface) GetStatus() bool {
	return i.materialized
}
