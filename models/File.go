package models

import (
	"database/sql"
	"os"

	"github.com/JojiiOfficial/gaw"
	"github.com/jinzhu/gorm"
	log "github.com/sirupsen/logrus"
)

//File a file uploaded to the db
type File struct {
	gorm.Model
	Name           string `gorm:"not null"`
	LocalName      string `gorm:"not null"`
	User           *User  `gorm:"association_autoupdate:false;association_autocreate:false"`
	UserID         uint   `gorm:"column:uploader;index"`
	FileSize       int64
	FileType       string
	IsPublic       bool           `gorm:"default:false"`
	PublicFilename sql.NullString `gorm:"unique"`
	Groups         []Group        `gorm:"many2many:files_groups;association_autoupdate:false"`
	Tags           []Tag          `gorm:"many2many:files_tags;association_autoupdate:false"`
	Namespace      *Namespace     `gorm:"association_autoupdate:false;association_autocreate:false"`
	NamespaceID    uint           `sql:"index" gorm:"not null"`
}

//FileAttributes attributes for a file
type FileAttributes struct {
	Tags      []string `json:"tags"`
	Groups    []string `json:"groups"`
	Namespace string   `json:"ns"`
}

//GetAttributes get file attributes
func (file File) GetAttributes() FileAttributes {
	return FileAttributes{
		Groups:    GroupArrToStringArr(file.Groups),
		Tags:      TagArrToStringArr(file.Tags),
		Namespace: file.GetNamespace().Name,
	}
}

//Insert inserts file into DB
func (file *File) Insert(db *gorm.DB, user *User) error {
	//Create groups
	for i := range file.Groups {
		if file.Groups[i].ID == 0 {
			if err := db.Where(&Group{
				Name: file.Groups[i].Name,
			}).Find(&file.Groups[i]).Error; err != nil {
				file.Groups[i].Insert(db, user)
			}
		}
	}

	//Create tags
	for i := range file.Tags {
		if file.Tags[i].ID == 0 {
			if err := db.Where(&Tag{
				Name: file.Tags[i].Name,
			}).Find(&file.Tags[i]).Error; err != nil {
				file.Tags[i].Insert(db, user)
			}
		}
	}

	//Use default namespace if not specified
	file.Namespace = file.GetNamespace()
	file.User = user

	//Create file
	if err := db.Create(file).Error; err != nil {
		return err
	}

	return nil
}

//GetNamespace return namespace of file
func (file File) GetNamespace() *Namespace {
	if file.Namespace == nil {
		return &DefaultNamespace
	}
	return file.Namespace
}

//IsInTagList return true if file has one of the specified tags
func (file File) IsInTagList(tags []Tag) bool {
	for _, tag := range file.Tags {
		for _, t1 := range tags {
			if tag.ID == t1.ID {
				return true
			}
		}
	}
	return false
}

//IsInGroupList return true if file is in one of the specified groups
func (file File) IsInGroupList(groups []Group) bool {
	for _, group := range file.Groups {
		for _, t1 := range groups {
			if group.ID == t1.ID {
				return true
			}
		}
	}
	return false
}

//FindFiles finds file
func FindFiles(db *gorm.DB, fileName string, namespace Namespace, user *User) ([]File, error) {
	var files []File
	a := db.Model(&File{}).Where("name = ? AND namespace_id = ? AND uploader = ?", fileName, namespace.ID, user.ID)

	//Get file to delete
	err := a.Preload("Namespace").Preload("Tags").Preload("Groups").Find(&files).Error
	if err != nil {
		return nil, err
	}

	return files, nil
}

//FindFile finds file
func FindFile(db *gorm.DB, fileName string, fileID uint, namespace Namespace, user *User) (*File, error) {
	a := db.Model(&File{}).Where("name = ? AND namespace_id = ? AND uploader = ?", fileName, namespace.ID, user.ID)
	if fileID != 0 {
		a = a.Where("id = ?", fileID)
	}

	//Get file to delete
	var file File
	err := a.Preload("Namespace").Preload("Tags").Preload("Groups").First(&file).Error
	if err != nil {
		return nil, err
	}

	return &file, nil
}

//HasTag return true if file is in group
func (file File) HasTag(sTag string) bool {
	for _, tag := range file.Tags {
		if tag.Name == sTag {
			return true
		}
	}
	return false
}

//HasGroup return true if file is in group
func (file File) HasGroup(sGroup string) bool {
	for _, group := range file.Groups {
		if group.Name == sGroup {
			return true
		}
	}
	return false
}

//Delete deletes a file
func (file *File) Delete(db *gorm.DB, config *Config) error {
	//Remove public filename to free this keyword
	file.IsPublic = false
	file.PublicFilename = sql.NullString{
		Valid: false,
	}

	//Save new state
	err := file.Save(db)
	if err != nil {
		return err
	}

	//Delete local file
	err = os.Remove(config.GetStorageFile(file.LocalName))
	if err != nil {
		log.Warn(err)
	}

	//Delete from DB
	return db.Delete(&file).Error
}

//Rename renames a file
func (file *File) Rename(db *gorm.DB, newName string) error {
	file.Name = newName
	return file.Save(db)
}

//SetVilibility sets public/private
func (file *File) SetVilibility(db *gorm.DB, newVisibility bool) error {
	file.IsPublic = newVisibility
	return file.Save(db)
}

//AddTags adds tags to file
func (file *File) AddTags(db *gorm.DB, tagsToAdd []string, user *User) error {
	for _, sTag := range tagsToAdd {
		if file.HasTag(sTag) {
			continue
		}

		tag := GetTag(db, sTag, file.Namespace, user)
		file.Tags = append(file.Tags, *tag)
	}
	return file.Save(db)
}

//RemoveTags remove tags to file
func (file *File) RemoveTags(db *gorm.DB, tagsToRemove []string) error {
	if len(file.Tags) == 0 {
		return nil
	}

	var newTags []Tag
	for i := range file.Tags {
		if !gaw.IsInStringArray(file.Tags[i].Name, tagsToRemove) {
			newTags = append(newTags, file.Tags[i])
		}
	}

	//Only save if at least one tag was removed
	if len(newTags) < len(file.Tags) {
		db.Model(&file).Association("Tags").Clear()
		file.Tags = newTags
		return file.Save(db)
	}

	return nil
}

//AddGroups adds groups to file
func (file *File) AddGroups(db *gorm.DB, groupsToAdd []string, user *User) error {
	for _, sGroup := range groupsToAdd {
		if file.HasGroup(sGroup) {
			continue
		}

		group := GetGroup(db, sGroup, file.Namespace, user)
		file.Groups = append(file.Groups, *group)
	}
	return file.Save(db)
}

//RemoveGroups remove groups from file
func (file *File) RemoveGroups(db *gorm.DB, groupsToRemove []string) error {
	if len(file.Groups) == 0 {
		return nil
	}

	var newGroups []Group
	for i := range file.Groups {
		if !gaw.IsInStringArray(file.Groups[i].Name, groupsToRemove) {
			newGroups = append(newGroups, file.Groups[i])
		}
	}

	//Only save if at least one group was removed
	if len(newGroups) < len(file.Groups) {
		db.Model(&file).Association("Groups").Clear()
		file.Groups = newGroups
		return file.Save(db)
	}
	return file.Save(db)
}

//Save saves a file in DB
func (file *File) Save(db *gorm.DB) error {
	return db.Save(file).Error
}

//GetCount get count if file
func (file File) GetCount(db *gorm.DB, fileID uint, user *User) (uint, error) {
	var c uint

	//Create count statement
	del := db.Model(&File{}).Where(&File{
		Name:        file.Name,
		NamespaceID: file.Namespace.ID,
		Model:       file.Model,
	}).Where("deleted_at is NULL")

	//Also use fileID if set
	if fileID != 0 {
		del = del.Where("id = ?", fileID)
	}

	//Execute statement
	err := del.Count(&c).Error

	return c, err
}

//GetPublicFile returns a file which is public
func GetPublicFile(db *gorm.DB, publicFilename string) (*File, bool, error) {
	var file File
	err := db.Model(&File{}).Where("public_filename = ?", publicFilename).First(&file).Error
	if err != nil {
		//Check error. Send server error if error is not "not found"
		if gorm.IsRecordNotFoundError(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	return &file, true, nil
}

//UpdateNamespace updates namespace for file
func (file *File) UpdateNamespace(db *gorm.DB, newNamespace *Namespace, user *User) error {
	//Set new namespace
	file.Namespace = newNamespace
	file.NamespaceID = newNamespace.ID

	//Update/move tags if available
	if len(file.Tags) > 0 {
		var newTags []Tag
		for _, tag := range file.Tags {
			newTag := GetTag(db, tag.Name, newNamespace, user)
			newTags = append(newTags, *newTag)
		}
		//remove old tags
		db.Model(&file).Association("Tags").Clear()
		//Set new tags
		file.Tags = newTags
	}

	//Update/move groups if available
	if len(file.Groups) > 0 {
		var newGroups []Group
		for _, group := range file.Groups {
			newGroup := GetGroup(db, group.Name, newNamespace, user)
			newGroups = append(newGroups, *newGroup)
		}
		//remove old groups
		db.Model(&file).Association("Groups").Clear()
		//Set new groups
		file.Groups = newGroups
	}

	//Save file
	return db.Save(&file).Error
}

//Publish publis a file
func (file *File) Publish(db *gorm.DB, publicName string) (bool, error) {
	//Determine public name
	if len(publicName) == 0 {
		publicName = gaw.RandString(25)
	}

	//Set file public name
	file.PublicFilename = sql.NullString{
		String: publicName,
		Valid:  true,
	}
	file.IsPublic = true

	//Check if public name already exists
	_, found, _ := GetPublicFile(db, publicName)
	if found {
		return true, nil
	}

	//Save new file
	return false, file.Save(db)
}
