/*
 * NRF Configuration Factory
 */

package factory

import (
	"fmt"
	"reflect"
	"io/ioutil"

	"gopkg.in/yaml.v2"

	"github.com/free5gc/nrf/logger"
)

var NrfConfig Config

// TODO: Support configuration update from REST api
func InitConfigFactory(f string) error {
	if content, err := ioutil.ReadFile(f); err != nil {
		return err
	} else {
		NrfConfig = Config{}

		if yamlErr := yaml.Unmarshal(content, &NrfConfig); yamlErr != nil {
			return yamlErr
		}
	}

	return nil
}

func UpdateNrfConfig(f string) error {
	if content, err := ioutil.ReadFile(f); err != nil {
		return err
	} else {
		var nrfConfig Config

		if yamlErr := yaml.Unmarshal(content, &nrfConfig); yamlErr != nil {
			return yamlErr
		}
		//Checking which config has been changed
		if reflect.DeepEqual(NrfConfig.Configuration.Sbi, nrfConfig.Configuration.Sbi) == false {
			logger.CfgLog.Infoln("Sbi Updated value ", nrfConfig.Configuration.Sbi)
		} 
		if reflect.DeepEqual(NrfConfig.Configuration.MongoDBName, nrfConfig.Configuration.MongoDBName) == false {
			logger.CfgLog.Infoln("MongoDBName Updated value ", nrfConfig.Configuration.MongoDBName)
		} 
		if reflect.DeepEqual(NrfConfig.Configuration.MongoDBUrl, nrfConfig.Configuration.MongoDBUrl) == false {
			logger.CfgLog.Infoln("MongoDBUrl Updated value ", nrfConfig.Configuration.MongoDBUrl)
		} 
		if reflect.DeepEqual(NrfConfig.Configuration.DefaultPlmnId, nrfConfig.Configuration.DefaultPlmnId) == false {
			logger.CfgLog.Infoln("DefaultPlmnId Updated value ", nrfConfig.Configuration.DefaultPlmnId)
		} 
		if reflect.DeepEqual(NrfConfig.Configuration.ServiceNameList, nrfConfig.Configuration.ServiceNameList) == false {
			logger.CfgLog.Infoln("ServiceNameList Updated value ", nrfConfig.Configuration.ServiceNameList)
		} 

		NrfConfig = nrfConfig
	}

	return nil
}

func CheckConfigVersion() error {
	currentVersion := NrfConfig.GetVersion()

	if currentVersion != NRF_EXPECTED_CONFIG_VERSION {
		return fmt.Errorf("config version is [%s], but expected is [%s].",
			currentVersion, NRF_EXPECTED_CONFIG_VERSION)
	}

	logger.CfgLog.Infof("config version [%s]", currentVersion)

	return nil
}
