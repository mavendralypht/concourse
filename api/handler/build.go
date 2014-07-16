package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	TurbineBuilds "github.com/concourse/turbine/api/builds"
	"github.com/pivotal-golang/lager"

	"github.com/concourse/atc/builds"
	"github.com/concourse/atc/config"
)

func (handler *Handler) UpdateBuild(w http.ResponseWriter, r *http.Request) {
	job := r.FormValue(":job")
	idStr := r.FormValue(":build")

	id, err := strconv.Atoi(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	build, err := handler.db.GetBuild(job, id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	var turbineBuild TurbineBuilds.Build
	if err := json.NewDecoder(r.Body).Decode(&turbineBuild); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log := handler.logger.Session("update-build", lager.Data{
		"job":    job,
		"id":     id,
		"status": turbineBuild.Status,
	})

	var status builds.Status

	switch turbineBuild.Status {
	case TurbineBuilds.StatusStarted:
		status = builds.StatusStarted
	case TurbineBuilds.StatusSucceeded:
		status = builds.StatusSucceeded
	case TurbineBuilds.StatusFailed:
		status = builds.StatusFailed
	case TurbineBuilds.StatusErrored:
		if build.Status == builds.StatusAborted {
			status = builds.StatusAborted
		} else {
			status = builds.StatusErrored
		}
	default:
		log.Info("unknown-status")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Info("save-status")

	err = handler.db.SaveBuildStatus(job, id, status)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	switch turbineBuild.Status {
	case TurbineBuilds.StatusStarted:
		for _, input := range turbineBuild.Inputs {
			err = handler.db.SaveBuildInput(job, id, buildInputFrom(input))
			if err != nil {
				log.Error("failed-to-save-input", err)
				w.WriteHeader(http.StatusInternalServerError)
			}
		}
	case TurbineBuilds.StatusSucceeded:
		for _, output := range turbineBuild.Outputs {
			err = handler.db.SaveOutputVersion(job, id, output.Name, builds.Version(output.Version))
			if err != nil {
				log.Error("failed-to-save-output-version", err)
				w.WriteHeader(http.StatusInternalServerError)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

func buildInputFrom(input TurbineBuilds.Input) builds.Input {
	metadata := make([]builds.MetadataField, len(input.Metadata))
	for i, md := range input.Metadata {
		metadata[i] = builds.MetadataField{
			Name:  md.Name,
			Value: md.Value,
		}
	}

	return builds.Input{
		Name:     input.Name,
		Source:   config.Source(input.Source),
		Version:  builds.Version(input.Version),
		Metadata: metadata,
	}
}
