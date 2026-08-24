package http

import (
	"io"

	"audiax/internal/constants"
	"audiax/internal/delivery/http/middleware"
	"audiax/internal/model"
	"audiax/internal/usecase"

	"github.com/gofiber/fiber/v2"
)

type BaselineController struct {
	useCase *usecase.BaselineUseCase
}

func NewBaselineController(useCase *usecase.BaselineUseCase) *BaselineController {
	return &BaselineController{useCase: useCase}
}

func (c *BaselineController) Calibrate(ctx *fiber.Ctx) error {
	audio, filename, err := readAudioUpload(ctx)
	if err != nil {
		return err
	}

	response, err := c.useCase.Calibrate(ctx.UserContext(), &model.CalibrateMachineRequest{
		UserID:    middleware.GetAuth(ctx).UserID,
		MachineID: ctx.Params("machineId"),
		Filename:  filename,
		Audio:     audio,
	})
	if err != nil {
		return err
	}
	return ctx.Status(fiber.StatusCreated).JSON(model.WebResponse[*model.BaselineResponse]{Data: response})
}

func (c *BaselineController) List(ctx *fiber.Ctx) error {
	response, err := c.useCase.List(ctx.UserContext(),
		middleware.GetAuth(ctx).UserID, ctx.Params("machineId"))
	if err != nil {
		return err
	}
	return ctx.JSON(model.WebResponse[[]model.BaselineResponse]{Data: response})
}

// readAudioUpload pulls the audio file out of a multipart request and returns
// its bytes. Reading it whole is deliberate: the same audio goes to the AI
// service and to object storage, and a reader can only be consumed once.
// constants.MaxAudioUploadBytes bounds the memory this can cost.
func readAudioUpload(ctx *fiber.Ctx) ([]byte, string, error) {
	header, err := ctx.FormFile(constants.AudioFormField)
	if err != nil {
		return nil, "", fiber.NewError(fiber.StatusBadRequest,
			"an audio file is required in the "+constants.AudioFormField+" field")
	}
	if header.Size > constants.MaxAudioUploadBytes {
		return nil, "", fiber.NewError(fiber.StatusRequestEntityTooLarge, "audio file is too large")
	}

	file, err := header.Open()
	if err != nil {
		return nil, "", fiber.NewError(fiber.StatusBadRequest, "audio file could not be read")
	}
	defer file.Close()

	audio, err := io.ReadAll(io.LimitReader(file, constants.MaxAudioUploadBytes))
	if err != nil {
		return nil, "", fiber.NewError(fiber.StatusBadRequest, "audio file could not be read")
	}
	if len(audio) == 0 {
		return nil, "", fiber.NewError(fiber.StatusBadRequest, "audio file is empty")
	}
	return audio, header.Filename, nil
}
