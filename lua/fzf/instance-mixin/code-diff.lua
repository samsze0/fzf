local FzfController = require("fzf.core.controller")
local config = require("fzf.core.config").value
local opts_utils = require("utils.opts")
local jumplist = require("jumplist")
local vimdiff_utils = require("utils.vimdiff")
local file_utils = require("utils.files")
local NuiEvent = require("nui.utils.autocmd").event
local winhighlight_utils = require("utils.winhighlight")
local oop_utils = require("utils.oop")

local _info = config.notifier.info
---@cast _info -nil
local _warn = config.notifier.warn
---@cast _warn -nil
local _error = config.notifier.error
---@cast _error -nil

---@class FzfCodeDiffLayout : FzfLayout
---@field side_popups { a: FzfSidePopup, b: FzfSidePopup }

---@alias FzfCodeDiffInstanceMixin.accessor fun(entry: FzfEntry): { filepath?: string, lines?: string[], filetype?: string }
---@alias FzfCodeDiffInstanceMixin.picker fun(entry: FzfEntry): ("a" | "b")

---@class FzfCodeDiffInstanceMixin : FzfController
---@field layout FzfCodeDiffLayout
---@field _a_accessor FzfCodeDiffInstanceMixin.accessor
---@field _b_accessor FzfCodeDiffInstanceMixin.accessor
---@field _picker FzfCodeDiffInstanceMixin.picker
local FzfCodeDiffInstanceMixin = oop_utils.new_class(FzfController)

-- Vim's default diff highlights doesn't align with the one used by git
--
---@param opts? { }
---@return WinHighlights, WinHighlights
function FzfCodeDiffInstanceMixin:setup_diff_highlights(opts)
  opts = opts_utils.extend({}, opts)

  local a_win_hl = {
    DiffAdd = "FzfDiffAddAsDelete",
    DiffDelete = "FzfDiffPadding",
    DiffChange = "FzfDiffChange",
    DiffText = "FzfDiffText",
  }

  local b_win_hl = {
    DiffAdd = "FzfDiffAdd",
    DiffDelete = "FzfDiffPadding",
    DiffChange = "FzfDiffChange",
    DiffText = "FzfDiffText",
  }

  return a_win_hl, b_win_hl
end

-- Configure file preview
--
---@param opts? { }
function FzfCodeDiffInstanceMixin:setup_filepreview(opts)
  opts = opts_utils.extend({}, opts)

  self:on_focus(function(payload)
    self.layout.side_popups.a:set_lines({})
    self.layout.side_popups.b:set_lines({})

    local focus = self.focus
    if not focus then return end

    local function show(x, popup)
      if x.filepath then
        popup:show_file_content(x.filepath)
      elseif x.lines then
        popup:set_lines(x.lines, {
          filetype = x.filetype,
        })
      end
    end

    local a = self._a_accessor(self.focus)
    show(a, self.layout.side_popups.a)

    local b = self._b_accessor(self.focus)
    show(b, self.layout.side_popups.b)

    vimdiff_utils.diff_bufs(
      self.layout.side_popups.a.bufnr,
      self.layout.side_popups.b.bufnr
    )
  end)
end

-- TODO: move to private config
function FzfCodeDiffInstanceMixin:setup_fileopen_keymaps()
  ---@param save_in_jumplist boolean
  ---@param open_command string
  local function open_file(save_in_jumplist, open_command)
    if not self.focus then return end

    local a_or_b = self._picker(self.focus)

    local function get_path(x)
      if x.filepath then
        return x.filepath
      else
        return file_utils.write_to_tmpfile(x.lines)
      end
    end

    local filepath
    local filetype
    if a_or_b == "a" then
      local a = self._a_accessor(self.focus)
      filepath = get_path(a)
      filetype = a.filetype
    else
      local b = self._b_accessor(self.focus)
      filepath = get_path(b)
      filetype = b.filetype
    end

    self:hide()
    if save_in_jumplist then jumplist.save() end
    vim.cmd(([[%s %s]]):format(open_command, filepath))

    if filetype then vim.bo.filetype = filetype end
  end

  self.layout.main_popup:map(
    "<C-w>",
    "Open in new window",
    function() open_file(false, "vsplit") end
  )

  self.layout.main_popup:map(
    "<C-t>",
    "Open in new tab",
    function() open_file(false, "tabnew") end
  )

  self.layout.main_popup:map(
    "<CR>",
    "Open",
    function() open_file(true, "edit") end
  )
end

-- TODO: move to private config
function FzfCodeDiffInstanceMixin:setup_copy_filepath_keymap()
  self.layout.main_popup:map("<C-y>", "Copy filepath", function()
    if not self.focus then return end

    local a_or_b = self._picker(self.focus)
    local filepath
    if a_or_b == "a" then
      local a = self._a_accessor(self.focus)
      if not a.filepath then
        _warn("No filepath accessor for a")
        return
      end
      filepath = a.filepath
    else
      local b = self._b_accessor(self.focus)
      if not b.filepath then
        _warn("No filepath accessor for b")
        return
      end
      filepath = b.filepath
    end

    vim.fn.setreg("+", filepath)
    _info(([[Copied %s to clipboard]]):format(filepath))
  end)
end

return FzfCodeDiffInstanceMixin