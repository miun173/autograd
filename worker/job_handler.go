package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/gocraft/work"
	"github.com/gomodule/redigo/redis"
	"github.com/miun173/autograd/model"
	"github.com/miun173/autograd/utils"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// job names
const (
	jobCheckAllDueAssignments string = "check_due_assignment"
	jobGradeAssignment        string = "grade_assignment"
	jobGradeSubmission        string = "grade_submission"
)

// Grader ..
type Grader interface {
	GradeSubmission(submissionID int64) error
}

// SubmissionUsecase ..
type SubmissionUsecase interface {
	FindAllUncheckByAssignmentID(assignmentID int64) (ids []int64, count int64, err error)
}

// AssignmentUsecase ..
type AssignmentUsecase interface {
	FindAllDueDates(c model.Cursor) (ids []int64, count int64, err error)
}

type jobHandler struct {
	pool       *work.WorkerPool
	redisPool  *redis.Pool
	enqueuer   *work.Enqueuer
	grader     Grader
	submission SubmissionUsecase
	assignment AssignmentUsecase
}

func (h *jobHandler) handleCheckAllDueAssignments(job *work.Job) error {
	logrus.Warn("start >>> ", time.Now())
	var size, page int64 = 10, 1
	cursor := model.NewCursor(size, page, model.SortCreatedAtDesc)
	idsChan := make(chan []int64)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	eg, ctx := errgroup.WithContext(ctx)
	defer cancel()

	// produce
	eg.Go(func() error {
		defer close(idsChan)
		for {
			ids, _, err := h.assignment.FindAllDueDates(cursor)
			if err != nil {
				logrus.Error(err)
				return fmt.Errorf("unable to get all due assignments: %w", err)
			}

			if len(ids) == 0 {
				break
			}

			idsChan <- ids

			page++
			cursor.SetPage(page)
		}

		return nil
	})

	// consume
	eg.Go(func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ids := <-idsChan:
			for _, id := range ids {
				logrus.Warn("process id >>> ", id)
				continue
				_, err := h.enqueuer.EnqueueUnique(jobGradeAssignment, work.Q{"assignmentID": id})
				return fmt.Errorf("unable to enqueue assignment %d: %w", id, err)
			}
		}

		return nil
	})

	err := eg.Wait()
	if err != nil && err != context.Canceled {
		logrus.Error(err)
		return err
	}

	logrus.Warn("done")
	return nil
}

func (h *jobHandler) handleGradeAssignment(job *work.Job) error {
	assignmentID := job.ArgInt64("assignmentID")
	ids, _, err := h.submission.FindAllUncheckByAssignmentID(assignmentID)
	if err != nil {
		return err
	}

	for _, id := range ids {
		arg := work.Q{"submissionID": id}
		if _, err := h.enqueuer.EnqueueUnique(jobGradeSubmission, arg); err != nil {
			logrus.Error(err)
			return err
		}
	}

	return nil
}

func (h *jobHandler) handleGradeSubmission(job *work.Job) error {
	submissionID := utils.StringToInt64(job.ArgString("submissionID"))
	return h.grader.GradeSubmission(submissionID)
}
