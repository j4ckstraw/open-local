/*
Copyright 2021 OECP Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cache

import (
	"fmt"
	"sync"

	"github.com/oecp/open-local-storage-service/pkg"
	lsstype "github.com/oecp/open-local-storage-service/pkg"
	nodelocalstorage "github.com/oecp/open-local-storage-service/pkg/apis/storage/v1alpha1"
	"github.com/oecp/open-local-storage-service/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	log "k8s.io/klog"
)

func NewNodeCache(nodeName string) *NodeCache {
	return &NodeCache{
		rwLock: sync.RWMutex{},
		NodeInfo: NodeInfo{NodeName: nodeName,
			VGs:          make(map[ResourceName]SharedResource, 0),
			MountPoints:  make(map[ResourceName]ExclusiveResource, 0),
			Devices:      make(map[ResourceName]ExclusiveResource, 0),
			AllocatedNum: 0,
			// TODO(yuzhi.wx) using pv name may conflict, use pv uid later
			LocalPVs: make(map[string]corev1.PersistentVolume, 0)},
	}
}

func NewNodeCacheFromStorage(nodeLocal *nodelocalstorage.NodeLocalStorage) *NodeCache {
	newNodeCache := NewNodeCache(nodeLocal.Name) // create a new node cache

	// VGs
	vgInfoMap := make(map[string]nodelocalstorage.VolumeGroup, len(nodeLocal.Status.FilteredStorageInfo.VolumeGroups))
	for _, vg := range nodeLocal.Status.NodeStorageInfo.VolumeGroups {
		vgInfoMap[vg.Name] = vg
	}
	// add vgs
	for _, vgName := range nodeLocal.Status.FilteredStorageInfo.VolumeGroups {
		log.V(5).Infof("adding new volume group %q(total:%d,allocatable:%d,used:%d) on node cache %s",
			vgName, vgInfoMap[vgName].Total, vgInfoMap[vgName].Allocatable, vgInfoMap[vgName].Total-vgInfoMap[vgName].Available, newNodeCache.NodeName)
		log.V(6).Infof("vg raw info:%#v", vgInfoMap[vgName])
		log.V(6).Infof("cachedNode.VGs: %#v, is nil %t", newNodeCache.VGs, newNodeCache.VGs == nil)
		vgResource := SharedResource{vgName, int64(vgInfoMap[vgName].Allocatable), 0}
		newNodeCache.VGs[ResourceName(vgName)] = vgResource
		log.V(6).Infof("vgResource: %#v", vgResource)
	}

	// Devices
	deviceInfoMap := make(map[string]nodelocalstorage.DeviceInfo, 0)
	for _, d := range nodeLocal.Status.NodeStorageInfo.DeviceInfos {
		deviceInfoMap[d.Name] = d
	}
	// add devices
	for _, deviceName := range nodeLocal.Status.FilteredStorageInfo.Devices {
		tmpDevice := deviceInfoMap[deviceName]
		log.V(5).Infof("adding new device %q(total:%d) on node cache %s",
			deviceName, deviceInfoMap[deviceName].Total, newNodeCache.NodeName)
		log.V(6).Infof("disk raw info:%#v", tmpDevice)
		diskResource := ExclusiveResource{
			tmpDevice.Name,
			tmpDevice.Name,
			int64(tmpDevice.Total),
			lsstype.MediaType(tmpDevice.MediaType),
			false}
		newNodeCache.Devices[ResourceName(deviceName)] = diskResource
		log.V(6).Infof("diskResource: %#v", diskResource)
	}

	// MountPoint
	mpInfoMap := make(map[string]nodelocalstorage.MountPoint, 0)
	for _, mp := range nodeLocal.Status.NodeStorageInfo.MountPoints {
		mpInfoMap[mp.Name] = mp
	}
	// add mount points
	for _, mp := range nodeLocal.Status.FilteredStorageInfo.MountPoints {
		tmpMP := mpInfoMap[mp]
		if utils.CheckMountPointOptions(&tmpMP) == false {
			continue
		}
		log.V(5).Infof("adding new mount point %q(total:%d) on node cache %s",
			mp, tmpMP.Total, newNodeCache.NodeName)
		log.V(6).Infof("disk raw info:%#v", tmpMP)
		diskResource := ExclusiveResource{
			mp,
			tmpMP.Device,
			int64(tmpMP.Total),
			lsstype.MediaType(deviceInfoMap[tmpMP.Device].MediaType),
			false}
		newNodeCache.MountPoints[ResourceName(mp)] = diskResource
		log.V(6).Infof("diskResource: %#v", diskResource)
	}
	return newNodeCache
}

func (nc *NodeCache) UpdateNodeInfo(nodeLocal *nodelocalstorage.NodeLocalStorage) *NodeCache {
	nc.rwLock.Lock()
	defer nc.rwLock.Unlock()
	// make a copy first, we may need make a deepcopy
	cacheNode := nc
	// VG
	// get vg from CR
	volumeGroups := nodeLocal.Status.NodeStorageInfo.VolumeGroups
	vgMapInfo := make(map[string]nodelocalstorage.VolumeGroup, 0)
	for _, vg := range volumeGroups {
		vgMapInfo[vg.Name] = vg
	}
	// get vg from cache
	vgCache := make([]string, 0)
	for _, vg := range cacheNode.VGs {
		vgCache = append(vgCache, vg.Name)
	}
	// update VGs
	addedVGs, unchangedVGs, removedVGs := utils.GetAddedAndRemovedItems(nodeLocal.Status.FilteredStorageInfo.VolumeGroups, vgCache)
	for _, vg := range addedVGs {
		log.V(5).Infof("adding new volume group %q(total:%d,allocatable:%d,used:%d) on node cache %s",
			vg, vgMapInfo[vg].Total, vgMapInfo[vg].Allocatable, vgMapInfo[vg].Total-vgMapInfo[vg].Available, cacheNode.NodeName)
		log.V(6).Infof("updatedName raw info:%#v", vgMapInfo[vg])
		log.V(6).Infof("cachedNode.VGs: %#v, is nil %t", cacheNode.VGs, cacheNode.VGs == nil)
		vgRequested := utils.GetVGRequested(nc.LocalPVs, vg)
		vgResource := SharedResource{vg, int64(vgMapInfo[vg].Allocatable), vgRequested}
		cacheNode.VGs[ResourceName(vg)] = vgResource
		log.V(6).Infof("vgResource: %#v", vgResource)
	}
	for _, vg := range unchangedVGs {
		// update the size if the updatedName got extended
		v, _ := cacheNode.VGs[ResourceName(vg)]
		v.Capacity = int64(vgMapInfo[vg].Allocatable)
		cacheNode.VGs[ResourceName(vg)] = v
		log.V(5).Infof("updating existing volume group %q(total:%d,allocatable:%d,used:%d) on node cache %s",
			vg, vgMapInfo[vg].Total, vgMapInfo[vg].Allocatable, vgMapInfo[vg].Total-vgMapInfo[vg].Available, cacheNode.NodeName)
	}
	for _, vg := range removedVGs {
		delete(cacheNode.VGs, ResourceName(vg))
		log.V(3).Infof("deleted vg %s from node cache %s", vg, nodeLocal.Name)
	}

	// Device
	// get device from CR
	devices := nodeLocal.Status.NodeStorageInfo.DeviceInfos
	deviceMapInfo := make(map[string]nodelocalstorage.DeviceInfo, 0)
	for _, d := range devices {
		deviceMapInfo[d.Name] = d
	}
	// get device from cache
	deviceCache := make([]string, 0)
	for _, device := range cacheNode.Devices {
		deviceCache = append(deviceCache, device.Name)
	}
	// update devices
	addedDevices, unchangedDevices, removedDevices := utils.GetAddedAndRemovedItems(nodeLocal.Status.FilteredStorageInfo.Devices, deviceCache)
	for _, device := range addedDevices {
		log.V(5).Infof("adding new device %q(total:%d) on node cache %s", device, deviceMapInfo[device].Total, cacheNode.NodeName)
		allocated := false
		if nc.IsLSSPVExist(pkg.VolumeTypeDevice, device) {
			allocated = true
		}
		diskResource := ExclusiveResource{
			device,
			device,
			int64(deviceMapInfo[device].Total),
			lsstype.MediaType(deviceMapInfo[device].MediaType),
			allocated}
		cacheNode.Devices[ResourceName(device)] = diskResource
	}
	for _, device := range unchangedDevices {
		// update the size if the device got extended
		exDevice, _ := cacheNode.Devices[ResourceName(device)]
		exDevice.Capacity = int64(deviceMapInfo[device].Total)
		exDevice.MediaType = lsstype.MediaType(deviceMapInfo[device].MediaType)
		cacheNode.Devices[ResourceName(device)] = exDevice
	}
	for _, device := range removedDevices {
		if cacheNode.Devices[ResourceName(device)].IsAllocated {
			log.Errorf("device %q is used by PV.", device)
		} else {
			delete(cacheNode.Devices, ResourceName(device))
			log.V(6).Infof("device %q has been deleted from cache", device)
		}
	}

	// MountPoint
	// get mountpoint from CR
	mountPoints := nodeLocal.Status.NodeStorageInfo.MountPoints
	mpMapInfo := make(map[string]nodelocalstorage.MountPoint, 0)
	for _, mp := range mountPoints {
		mpMapInfo[mp.Name] = mp
	}
	// get mountpoint from cache
	cacheMP := make([]string, 0)
	for _, mp := range cacheNode.MountPoints {
		cacheMP = append(cacheMP, mp.Name)
	}
	// update mount points
	addedMPs, unchangedMPs, removedMPs := utils.GetAddedAndRemovedItems(nodeLocal.Status.FilteredStorageInfo.MountPoints, cacheMP)
	for _, mp := range addedMPs {
		tmpMP := mpMapInfo[mp]
		if utils.CheckMountPointOptions(&tmpMP) == false {
			log.Warningf("mount point %s on %s was excluded, readonly: %t, fsType: %s", mp, nodeLocal.Name, tmpMP.ReadOnly, tmpMP.FsType)
			continue
		}
		log.V(5).Infof("adding new mount point %q(total:%d) on node cache %s", mp, mpMapInfo[mp].Total, cacheNode.NodeName)
		allocated := false
		if nc.IsLSSPVExist(pkg.VolumeTypeMountPoint, mp) {
			allocated = true
		}
		diskResource := ExclusiveResource{
			mp,
			mpMapInfo[mp].Device,
			int64(mpMapInfo[mp].Total),
			lsstype.MediaType(deviceMapInfo[mpMapInfo[mp].Device].MediaType),
			allocated}
		cacheNode.MountPoints[ResourceName(mp)] = diskResource
		log.V(6).Infof("diskResource: %#v", diskResource)
	}
	for _, mp := range unchangedMPs {
		exMP, _ := cacheNode.MountPoints[ResourceName(mp)]
		// update capacity of existing mount point
		exMP.Capacity = int64(mpMapInfo[mp].Total)
		exMP.MediaType = lsstype.MediaType(deviceMapInfo[exMP.Device].MediaType)
		cacheNode.MountPoints[ResourceName(mp)] = exMP
		log.V(6).Infof("updating existing mount point %q(total:%d) on node cache %s",
			exMP.Name, exMP.Capacity, cacheNode.NodeName)
	}
	for _, mp := range removedMPs {
		if cacheNode.MountPoints[ResourceName(mp)].IsAllocated {
			log.Errorf("mount point %q is used by PV.", mp)
		} else {
			delete(cacheNode.MountPoints, ResourceName(mp))
			log.V(6).Infof("mount point %q has been deleted from cache", mp)
		}
	}

	return cacheNode
}

// AddLVM add lvm PV to cache
// note: this function does not handle pv update event
func (nc *NodeCache) AddLVM(pv *corev1.PersistentVolume) error {
	if !nc.isNodeLocal(pv) {
		return nil
	}
	nc.rwLock.Lock()
	defer nc.rwLock.Unlock()
	vgName := utils.GetVGNameFromCsiPV(pv)
	if len(vgName) == 0 {
		log.V(5).Infof("pv %s is not bound to any volume group, skipped", pv.Name)
		return nil
	} else {
		existing, ok := nc.LocalPVs[pv.Name]
		if ok {
			if existing.UID == pv.UID {
				log.V(5).Infof("pv %s(uid=%s) was already existed", pv.Name, pv.UID)
			}
		}
		if vg, ok := nc.VGs[ResourceName(vgName)]; ok {
			// TODO(huizhi.szh): when informer resync the cache, this function may be called again, this will be a bug,
			// because it will do it one more time.
			oldRequest := vg.Requested
			s := pv.Spec.Capacity[corev1.ResourceStorage]
			vg.Requested = oldRequest + s.Value()
			// Added to node cache
			nc.AllocatedNum += 1
			nc.VGs[ResourceName(vgName)] = vg
			log.Infof("[AddLVM]added pv %s: VG info: old size => %d, new size => %d for vg %s ",
				pv.Name, oldRequest, vg.Requested, vgName)
		} else {
			// ideally, this path should never be reached
			// log.Errorf("[AddLVM]no vg %s found in for node %s when adding pv %s", vgName, nc.NodeName, pv.Name)
			nc.AllocatedNum += 1
			log.Infof("[AddLVM]vg %s not found in NodeCache", vgName)
		}
		nc.LocalPVs[pv.Name] = *pv
	}

	return nil
}

// UpdateLVM updates lvm PV to cache
// note: this function does not handle pv add event
func (nc *NodeCache) UpdateLVM(old, pv *corev1.PersistentVolume) error {
	if !nc.isNodeLocal(pv) {
		return nil
	}
	nc.rwLock.Lock()
	defer nc.rwLock.Unlock()
	vgName := utils.GetVGNameFromCsiPV(pv)
	if len(vgName) == 0 {
		log.V(5).Infof("pv %s is not a valid open-local-storage-service lvm pv", pv.Name)
	} else {
		existing, ok := nc.LocalPVs[pv.Name]
		if ok {
			if existing.UID == pv.UID {
				log.V(5).Infof("pv %s(uid=%s) was already existed", pv.Name, pv.UID)
				nc.LocalPVs[pv.Name] = *pv
				return nil
			}
		}
		if vg, ok := nc.VGs[ResourceName(vgName)]; ok {
			// because it is already in cache, we only recalculate vg requested size and PV object
			oldRequest := vg.Requested
			newPVsize := pv.Spec.Capacity[corev1.ResourceStorage]
			oldPVsize := old.Spec.Capacity[corev1.ResourceStorage]
			vg.Requested = oldRequest - oldPVsize.Value() + newPVsize.Value()
			nc.VGs[ResourceName(vgName)] = vg
			log.Infof("[UpdateLVM]updated pv %s: VG info: old size => %d, new size => %d for vg %s ",
				pv.Name, oldRequest, vg.Requested, vgName)
		} else {
			// ideally, this path should never be reached
			// log.Errorf("[UpdateLVM]no vg %s found in node cache when updating pv %s", vgName, pv.Name)
			nc.AllocatedNum += 1
			log.Infof("[UpdateLVM]vg %s not found in NodeCache", vgName)
		}
		nc.LocalPVs[pv.Name] = *pv
	}

	return nil
}

func (nc *NodeCache) RemoveLVM(pv *corev1.PersistentVolume) error {
	if !nc.isNodeLocal(pv) {
		return nil
	}
	nc.rwLock.Lock()
	defer nc.rwLock.Unlock()
	// Hardcode to volume group named vg
	vgName := utils.GetVGNameFromCsiPV(pv)
	if len(vgName) == 0 {
		log.V(5).Infof("pv %s is not a valid open-local-storage-service pv(lvm with name)", pv.Name)
	}
	if vg, ok := nc.VGs[ResourceName(vgName)]; ok {
		oldUsed := vg.Requested
		s := pv.Spec.Capacity[corev1.ResourceStorage]
		vg.Requested = oldUsed - s.Value()
		nc.AllocatedNum -= 1
		nc.VGs[ResourceName(vgName)] = vg
		log.Infof("[RemoveLVM]removed pv %s: VG info: old size => %d, new size => %d for vg %s ", pv.Name, oldUsed, vg.Requested, vgName)
	} else {
		nc.AllocatedNum -= 1
		log.Infof("[RemoveLVM]pv %s was not in the node cache, skipped updating", pv.Name)
	}
	delete(nc.LocalPVs, pv.Name)
	return nil
}

func (nc *NodeCache) AddLocalMountPoint(pv *corev1.PersistentVolume) error {
	if !nc.isNodeLocal(pv) {
		return nil
	}

	nc.rwLock.Lock()
	defer nc.rwLock.Unlock()
	diskName := utils.GetMountPointFromCsiPV(pv)
	if len(diskName) == 0 {
		log.V(5).Infof("pv %s is not a valid open-local-storage-service pv(disk with name)", pv.Name)
	} else {
		if disk, ok := nc.MountPoints[ResourceName(diskName)]; ok {
			if disk.IsAllocated == false {
				disk.IsAllocated = true
				nc.AllocatedNum += 1
				nc.MountPoints[ResourceName(diskName)] = disk
				log.Infof("[AddLocalMountPoint]added pv %s: disk info: label it allocated for disk %s", pv.Name, disk.Name)
			}
		} else {
			nc.AllocatedNum += 1
			log.Infof("[AddLocalMountPoint]disk %s not found in NodeCache", diskName)
		}
		nc.LocalPVs[pv.Name] = *pv
	}

	return nil
}

func (nc *NodeCache) RemoveLocalMountPoint(pv *corev1.PersistentVolume) error {
	if !nc.isNodeLocal(pv) {
		return nil
	}
	nc.rwLock.Lock()
	defer nc.rwLock.Unlock()
	diskName := utils.GetMountPointFromCsiPV(pv)
	if len(diskName) == 0 {
		log.V(5).Infof("pv %s is not a valid open-local-storage-service pv(mount point with name)", pv.Name)
	}
	if disk, ok := nc.MountPoints[ResourceName(diskName)]; ok {
		disk.IsAllocated = false
		nc.AllocatedNum -= 1
		nc.MountPoints[ResourceName(diskName)] = disk
		log.Infof("[RemoveLocalMountPoint]removed pv %s: disk info: label it not allocated for disk %s", pv.Name, disk.Name)
	} else {
		nc.AllocatedNum -= 1
		log.Infof("[RemoveLocalMountPoint]pv %s was not in the node cache, skipped updating", pv.Name)
	}
	delete(nc.LocalPVs, pv.Name)
	return nil
}

func (nc *NodeCache) AddLocalDevice(pv *corev1.PersistentVolume) error {
	if !nc.isNodeLocal(pv) {
		return nil
	}
	nc.rwLock.Lock()
	defer nc.rwLock.Unlock()
	deviceName := utils.GetDeviceNameFromCsiPV(pv)
	if len(deviceName) == 0 {
		err := fmt.Errorf("pv %s is not a valid open-local-storage-service pv(device with name)", pv.Name)
		return err
	} else {
		if device, ok := nc.Devices[ResourceName(deviceName)]; ok {
			if device.IsAllocated == false {
				device.IsAllocated = true
				nc.AllocatedNum += 1
				nc.Devices[ResourceName(deviceName)] = device
				log.Infof("[AddLocalDevice]added pv %s: device info: label it allocated for device %s", pv.Name, device.Name)
			}
		} else {
			log.Infof("[AddLocalDevice]device %s not found in NodeCache(%s)", deviceName, nc.NodeName)
			nc.AllocatedNum += 1
		}
		nc.LocalPVs[pv.Name] = *pv
	}
	return nil
}

func (nc *NodeCache) RemoveLocalDevice(pv *corev1.PersistentVolume) error {
	if !nc.isNodeLocal(pv) {
		return nil
	}
	nc.rwLock.Lock()
	defer nc.rwLock.Unlock()
	deviceName := utils.GetDeviceNameFromCsiPV(pv)
	if len(deviceName) == 0 {
		log.V(5).Infof("pv %s is not a valid open-local-storage-service pv(device with name)", pv.Name)
	}
	if device, ok := nc.Devices[ResourceName(deviceName)]; ok {
		device.IsAllocated = false
		nc.AllocatedNum -= 1
		nc.Devices[ResourceName(deviceName)] = device
		log.Infof("[RemoveLocalDevice]removed pv %s: device info: label it not allocated for device %s", pv.Name, device.Name)
	} else {
		nc.AllocatedNum -= 1
		log.Infof("[RemoveLocalDevice]pv %s was not in the node cache, skipped updating", pv.Name)
	}
	delete(nc.LocalPVs, pv.Name)
	return nil
}

// isNodeLocal tests whether a PV is a local PV and belongs to this node
// it use label "kubernetes.io/hostname" to identify a node
func (nc *NodeCache) isNodeLocal(pv *corev1.PersistentVolume) bool {
	b, node := utils.IsLocalPV(pv)
	if b && node == nc.NodeName {
		return true
	}
	return false
}

// IsLSSPVExist will check whether PV exists in LocalPV according to resourceName and kind
func (nc *NodeCache) IsLSSPVExist(kind pkg.VolumeType, resourceName string) bool {
	for _, pv := range nc.LocalPVs {
		attributes := pv.Spec.CSI.VolumeAttributes
		if kind, exist := attributes[pkg.VolumeTypeKey]; exist {
			if lsstype, err := pkg.VolumeTypeFromString(kind); err == nil {
				var name string
				switch lsstype {
				case pkg.VolumeTypeMountPoint:
					name, exist = attributes[pkg.MPName]
				case pkg.VolumeTypeDevice:
					name, exist = attributes[pkg.DeviceName]
				case pkg.VolumeTypeLVM:
					name, exist = attributes[pkg.VGName]
				default:
					exist = false
				}
				if exist && name == resourceName {
					return true
				}
			}
		}
	}

	return false
}